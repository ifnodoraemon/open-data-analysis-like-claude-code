"""
Python Executor MCP Server
提供安全的 Python 代码执行环境，作为数据分析智能体的通用计算扩展。
通过 HTTP API 接收代码，在受限环境中执行，返回 stdout/stderr/文件输出。

安全策略（纵深防御）：
1. 进程级隔离：每个请求在独立子进程中执行
2. AST 预检：在执行前静态拒绝危险语法（importlib/ctypes/dunder属性链等）
3. Import 白名单：仅允许预声明安全的模块
4. 属性访问拦截：阻止 __class__/__bases__/__subclasses__/__globals__ 等逃逸链
5. 资源限制：超时上限、进程数限制、SIGKILL 后备
6. 文件系统隔离：open() 限制在工作目录内
"""

import ast
import io
import logging
import os
import json
import time
import traceback
import contextlib
import resource
import shutil
import uuid
import multiprocessing
import sys
from pathlib import Path

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import FileResponse
from pydantic import BaseModel, field_validator

app = FastAPI(title="Python Executor MCP", version="2.0.0")
logger = logging.getLogger("python-executor")
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")

WORK_DIR = Path(os.environ.get("WORK_DIR", "/app/workspace"))
WORK_DIR.mkdir(parents=True, exist_ok=True)

MAX_TIMEOUT = 120
MAX_CODE_SIZE = 65536

FORBIDDEN_IMPORTS = frozenset({
    'os', 'sys', 'subprocess', 'shutil', 'socket', 'urllib', 'requests',
    'pty', 'builtins', 'importlib', 'ctypes', 'signal', 'multiprocessing',
    'threading', 'pickle', 'shelve', 'marshal', 'code', 'codeop',
    'compileall', 'runpy', 'webbrowser', 'antigravity', 'site', 'pkgutil',
    'xmlrpc', 'http', 'ftplib', 'smtplib', 'telnetlib', 'tempfile',
    'glob', 'io', 'pathlib', 'posixpath', 'ntpath', 'genericpath',
})

ALLOWED_IMPORTS = frozenset({
    'pandas', 'numpy', 'matplotlib', 'scipy', 'sklearn', 'scikit-learn',
    'json', 'csv', 'math', 'statistics', 're', 'datetime', 'collections',
    'openpyxl', 'numbers', 'decimal', 'fractions', 'itertools',
    'functools', 'operator', 'copy', 'enum', 'typing', 'dataclasses',
    'string', 'textwrap', 'difflib', 'hashlib', 'base64', 'random',
    'uuid', 'pprint', 'traceback', 'warnings', 'contextlib',
    'unittest.mock',
})

DANGEROUS_ATTR_NAMES = frozenset({
    '__class__', '__bases__', '__base__', '__mro__', '__subclasses__',
    '__globals__', '__builtins__', '__code__', '__func__',
    '__self__', '__dict__', '__closure__', '__frame__',
    '__module__', '__qualname__', '__init__', '__new__',
})


class ASTSandboxValidator(ast.NodeVisitor):
    """AST 级别的静态安全检查，在执行前拒绝危险代码模式。"""

    def __init__(self):
        self.errors = []

    def _error(self, node, msg):
        self.errors.append(f"Line {getattr(node, 'lineno', '?')}: {msg}")

    def visit_Import(self, node):
        for alias in node.names:
            base = alias.name.split('.')[0]
            if base in FORBIDDEN_IMPORTS:
                self._error(node, f"import '{base}' is not allowed")
            elif base not in ALLOWED_IMPORTS:
                self._error(node, f"import '{base}' is not in the allowed list")
        self.generic_visit(node)

    def visit_ImportFrom(self, node):
        if node.module:
            base = node.module.split('.')[0]
            if base in FORBIDDEN_IMPORTS:
                self._error(node, f"from '{base}' import is not allowed")
            elif base not in ALLOWED_IMPORTS:
                self._error(node, f"from '{base}' import is not in the allowed list")
        self.generic_visit(node)

    def visit_Attribute(self, node):
        if node.attr in DANGEROUS_ATTR_NAMES:
            self._error(node, f"access to attribute '{node.attr}' is not allowed")
        self.generic_visit(node)

    def visit_Call(self, node):
        if isinstance(node.func, ast.Attribute) and node.func.attr in ('__import__', 'eval', 'exec', 'compile', 'open'):
            self._error(node, f"calling '{node.func.attr}' is not allowed in this context")
        if isinstance(node.func, ast.Name) and node.func.id in ('eval', 'exec', 'compile'):
            self._error(node, f"calling '{node.func.id}' is not allowed")
        self.generic_visit(node)


def validate_code_with_ast(code: str) -> list[str]:
    try:
        tree = ast.parse(code)
    except SyntaxError as e:
        return [f"Syntax error: {e}"]
    validator = ASTSandboxValidator()
    validator.visit(tree)
    return validator.errors


def init_namespace(ns, work_dir: str):
    imports = f"""
import pandas as pd
import numpy as np
import json
import csv
import math
import statistics
import re
from datetime import datetime, timedelta
from collections import Counter, defaultdict
from numbers import Number
from decimal import Decimal
from fractions import Fraction
import itertools
import functools
import operator
import copy
import enum
import string
import textwrap
import hashlib
import base64
import random
import warnings
import contextlib

import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
plt.rcParams['font.sans-serif'] = ['SimHei', 'DejaVu Sans']
plt.rcParams['axes.unicode_minus'] = False

WORK_DIR = '{work_dir}'
"""
    try:
        exec(imports, ns)
    except Exception as e:
        print(f"Warning: Some libraries not available: {e}")


class ExecuteRequest(BaseModel):
    code: str
    timeout: int = 30

    @field_validator('timeout')
    @classmethod
    def clamp_timeout(cls, v):
        return max(5, min(v, MAX_TIMEOUT))

    @field_validator('code')
    @classmethod
    def validate_code_size(cls, v):
        if len(v) > MAX_CODE_SIZE:
            raise ValueError(f"Code exceeds maximum size of {MAX_CODE_SIZE} bytes")
        return v


class ExecuteResponse(BaseModel):
    success: bool
    stdout: str
    stderr: str
    error: str | None = None
    files: list[str] = []
    duration_ms: int = 0


@app.middleware("http")
async def quiet_health_logs(request: Request, call_next):
    start = time.time()
    response = await call_next(request)
    path = request.url.path
    if path not in {"/health", "/tools"}:
        logger.info(
            "http method=%s path=%s status=%s duration_ms=%d",
            request.method,
            path,
            response.status_code,
            int((time.time() - start) * 1000),
        )
    return response


@app.get("/health")
def health():
    return {"status": "ok", "tools": ["code_run_python"]}


@app.get("/tools")
def list_tools():
    return {
        "tools": [
            {
                "name": "code_run_python",
                "description": (
                    "在安全的 Python 环境中执行代码。预装了 pandas, numpy, matplotlib, scipy。"
                    "可以进行数据处理、统计分析、机器学习、绘图等操作。"
                    "生成的图表文件会保存在工作目录中。"
                ),
                "parameters": {
                    "type": "object",
                    "properties": {
                        "code": {
                            "type": "string",
                            "description": "要执行的 Python 代码"
                        },
                        "timeout": {
                            "type": "integer",
                            "description": f"超时时间（秒），默认 30，最大 {MAX_TIMEOUT}",
                            "default": 30
                        }
                    },
                    "required": ["code"]
                }
            }
        ]
    }


def run_in_process(code: str, req_dir_path: str, q: multiprocessing.Queue):
    req_dir = Path(req_dir_path)
    os.chdir(req_dir)

    local_ns = {}
    init_namespace(local_ns, str(req_dir))

    import builtins
    safe_builtins = builtins.__dict__.copy()

    original_import = builtins.__import__

    def safe_import(name, globals=None, locals=None, fromlist=(), level=0):
        base_name = name.split('.')[0]
        if base_name in FORBIDDEN_IMPORTS:
            raise ImportError(f"Importing '{base_name}' is not allowed in this sandbox.")
        if base_name not in ALLOWED_IMPORTS:
            raise ImportError(f"Importing '{base_name}' is not allowed. Only pre-approved libraries are available.")
        return original_import(name, globals, locals, fromlist, level)

    _builtin_open = builtins.open

    def safe_open(file, mode='r', *args, **kwargs):
        resolved = Path(file).resolve()
        if not str(resolved).startswith(str(req_dir.resolve())):
            raise PermissionError("Access denied: file operations are restricted to the working directory.")
        if 'w' in mode or 'a' in mode or '+' in mode:
            pass
        return _builtin_open(resolved, mode, *args, **kwargs)

    safe_builtins['__import__'] = safe_import
    safe_builtins['open'] = safe_open
    safe_builtins.pop('eval', None)
    safe_builtins.pop('exec', None)
    safe_builtins.pop('compile', None)

    def safe_getattr(obj, name, *args):
        if isinstance(name, str) and name in DANGEROUS_ATTR_NAMES:
            raise AttributeError(f"Access to attribute '{name}' is not allowed in this sandbox.")
        return original_getattr(obj, name, *args)

    original_getattr = builtins.getattr
    safe_builtins['getattr'] = safe_getattr
    local_ns['__builtins__'] = safe_builtins

    try:
        import resource as _resource
        _resource.setrlimit(_resource.RLIMIT_AS, (512 * 1024 * 1024, 512 * 1024 * 1024))
        _resource.setrlimit(_resource.RLIMIT_NPROC, (0, 0))
        _resource.setrlimit(_resource.RLIMIT_FSIZE, (50 * 1024 * 1024, 50 * 1024 * 1024))
    except Exception:
        pass

    stdout_buf = io.StringIO()
    stderr_buf = io.StringIO()
    success = True
    error = None

    try:
        with contextlib.redirect_stdout(stdout_buf), contextlib.redirect_stderr(stderr_buf):
            exec(code, local_ns)
    except Exception as e:
        success = False
        error = f"{type(e).__name__}: {e}\n{traceback.format_exc()}"

    try:
        import matplotlib.pyplot as plt
        plt.close('all')
    except Exception:
        pass

    q.put({
        "success": success,
        "stdout": stdout_buf.getvalue(),
        "stderr": stderr_buf.getvalue(),
        "error": error
    })


@app.post("/execute", response_model=ExecuteResponse)
def execute_code(req: ExecuteRequest):
    start = time.time()

    ast_errors = validate_code_with_ast(req.code)
    if ast_errors:
        return ExecuteResponse(
            success=False,
            stdout="",
            stderr="",
            error="Code rejected by security policy:\n" + "\n".join(ast_errors),
            files=[],
            duration_ms=int((time.time() - start) * 1000),
        )

    req_id = f"req_{uuid.uuid4().hex[:8]}"
    req_dir = WORK_DIR / req_id
    req_dir.mkdir(parents=True, exist_ok=True)

    q = multiprocessing.Queue()
    p = multiprocessing.Process(target=run_in_process, args=(req.code, str(req_dir), q))
    p.start()

    p.join(req.timeout)

    timeout_occurred = False
    if p.is_alive():
        p.terminate()
        p.join(5)
        if p.is_alive():
            p.kill()
            p.join(5)
        timeout_occurred = True

    duration_ms = int((time.time() - start) * 1000)

    if timeout_occurred:
        shutil.rmtree(req_dir, ignore_errors=True)
        return ExecuteResponse(
            success=False,
            stdout="",
            stderr="",
            error=f"Execution timed out after {req.timeout} seconds.",
            files=[],
            duration_ms=duration_ms,
        )

    try:
        result = q.get_nowait()
    except Exception:
        result = {
            "success": False,
            "stdout": "",
            "stderr": "",
            "error": "Execution failed to return a result (possible crash or out of memory).",
        }

    new_files = []
    if req_dir.exists():
        for f in req_dir.glob("*"):
            if f.is_file():
                dest_name = f"{req_id}_{f.name}"
                shutil.move(str(f), str(WORK_DIR / dest_name))
                new_files.append(dest_name)
        shutil.rmtree(req_dir, ignore_errors=True)

    logger.info(
        "execute success=%s duration_ms=%d files=%d stdout_chars=%d stderr_chars=%d",
        result["success"],
        duration_ms,
        len(new_files),
        len(result["stdout"]),
        len(result["stderr"]),
    )

    return ExecuteResponse(
        success=result["success"],
        stdout=result["stdout"][:10000],
        stderr=result["stderr"][:5000],
        error=result["error"],
        files=new_files,
        duration_ms=duration_ms,
    )


@app.get("/files/{filename}")
def get_file(filename: str, request: Request):
    token = request.headers.get("X-Proxy-Token")
    expected_token = os.environ.get("PROXY_TOKEN")
    if not expected_token:
        raise HTTPException(503, "PROXY_TOKEN not configured; file access disabled")
    if token != expected_token:
        raise HTTPException(403, "Forbidden: Missing or invalid proxy token")

    filepath = (WORK_DIR / filename).resolve()
    if not str(filepath).startswith(str(WORK_DIR.resolve())):
        raise HTTPException(400, "Invalid file path")
    if not filepath.exists():
        raise HTTPException(404, f"File not found: {filename}")
    return FileResponse(filepath)


def _cleanup_old_files(max_age_hours: int = 24):
    """Remove workspace files older than max_age_hours to prevent disk exhaustion."""
    cutoff = time.time() - max_age_hours * 3600
    for f in WORK_DIR.glob("*"):
        if f.is_file() and f.stat().st_mtime < cutoff:
            try:
                f.unlink()
            except OSError:
                pass


@app.on_event("startup")
def on_startup():
    _cleanup_old_files()


if __name__ == "__main__":
    import uvicorn
    port = int(os.environ.get("PORT", "8081"))
    uvicorn.run(app, host="0.0.0.0", port=port, access_log=False)
