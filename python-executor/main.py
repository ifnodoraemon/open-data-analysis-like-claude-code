"""
Python Executor MCP Server
提供安全的 Python 代码执行环境，作为数据分析智能体的通用计算扩展。
通过 HTTP API 接收代码，在受限环境中执行，返回 stdout/stderr/文件输出。
"""

import io
import logging
import os
import json
import time
import traceback
import contextlib
from pathlib import Path

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import FileResponse
from pydantic import BaseModel

app = FastAPI(title="Python Executor MCP", version="1.0.0")
logger = logging.getLogger("python-executor")
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")

# 工作目录：存放生成的文件（图表等）
WORK_DIR = Path(os.environ.get("WORK_DIR", "/app/workspace"))
WORK_DIR.mkdir(parents=True, exist_ok=True)

def init_namespace(ns, work_dir: Path):
    """初始化命名空间，预加载常用库"""
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
from pathlib import Path

# matplotlib 非交互模式
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
plt.rcParams['font.sans-serif'] = ['SimHei', 'DejaVu Sans']
plt.rcParams['axes.unicode_minus'] = False

WORK_DIR = Path('{work_dir}')
"""
    try:
        exec(imports, ns)
    except Exception as e:
        print(f"Warning: Some libraries not available: {e}")


class ExecuteRequest(BaseModel):
    code: str
    timeout: int = 30  # 秒


class ExecuteResponse(BaseModel):
    success: bool
    stdout: str
    stderr: str
    error: str | None = None
    files: list[str] = []  # 生成的文件路径
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
    """MCP 工具列表"""
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
                            "description": "超时时间（秒），默认 30",
                            "default": 30
                        }
                    },
                    "required": ["code"]
                }
            }
        ]
    }


import multiprocessing
import sys
import shutil
import uuid

def run_in_process(code: str, req_dir_path: str, q: multiprocessing.Queue):
    req_dir = Path(req_dir_path)
    os.chdir(req_dir)
    
    local_ns = {}
    init_namespace(local_ns, req_dir)
    
    # 构建沙箱 builtins
    import builtins
    safe_builtins = builtins.__dict__.copy()
    
    original_import = builtins.__import__
    def safe_import(name, globals=None, locals=None, fromlist=(), level=0):
        forbidden = {'os', 'sys', 'subprocess', 'shutil', 'socket', 'urllib', 'requests', 'pty', 'builtins'}
        base_name = name.split('.')[0]
        if base_name in forbidden:
            raise ImportError(f"Importing '{base_name}' is not allowed in this sandbox.")
        return original_import(name, globals, locals, fromlist, level)
        
    safe_builtins['__import__'] = safe_import
    safe_builtins.pop('eval', None)
    safe_builtins.pop('exec', None)
    
    local_ns['__builtins__'] = safe_builtins

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
    except:
        pass

    q.put({
        "success": success,
        "stdout": stdout_buf.getvalue(),
        "stderr": stderr_buf.getvalue(),
        "error": error
    })


@app.post("/execute", response_model=ExecuteResponse)
def execute_code(req: ExecuteRequest):
    """执行 Python 代码"""
    start = time.time()
    
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
        p.join()
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
            duration_ms=duration_ms
        )

    try:
        result = q.get_nowait()
    except Exception:
        result = {
            "success": False,
            "stdout": "",
            "stderr": "",
            "error": "Execution failed to return a result (possible crash or out of memory)."
        }

    # 移动文件到 WORK_DIR 并返回列表
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
def get_file(filename: str):
    """获取生成的文件"""
    filepath = WORK_DIR / filename
    if not filepath.exists():
        raise HTTPException(404, f"File not found: {filename}")
    return FileResponse(filepath)


if __name__ == "__main__":
    import uvicorn
    port = int(os.environ.get("PORT", "8081"))
    uvicorn.run(app, host="0.0.0.0", port=port, access_log=False)
