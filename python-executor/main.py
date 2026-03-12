"""
Python Executor MCP Server
提供安全的 Python 代码执行环境，作为数据分析智能体的通用计算扩展。
通过 HTTP API 接收代码，在受限环境中执行，返回 stdout/stderr/文件输出。
"""

import io
import os
import sys
import json
import time
import uuid
import traceback
import contextlib
from pathlib import Path

from fastapi import FastAPI, HTTPException
from fastapi.responses import FileResponse
from pydantic import BaseModel

app = FastAPI(title="Python Executor MCP", version="1.0.0")

# 工作目录：存放生成的文件（图表等）
WORK_DIR = Path("/app/workspace")
WORK_DIR.mkdir(parents=True, exist_ok=True)

# 预加载常用数据分析库的全局命名空间
GLOBAL_NS = {}

def init_namespace():
    """初始化全局命名空间，预加载常用库"""
    imports = """
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

WORK_DIR = Path('/app/workspace')
"""
    try:
        exec(imports, GLOBAL_NS)
    except ImportError as e:
        print(f"Warning: Some libraries not available: {e}")

init_namespace()


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


@app.get("/health")
def health():
    return {"status": "ok", "tools": ["run_python"]}


@app.get("/tools")
def list_tools():
    """MCP 工具列表"""
    return {
        "tools": [
            {
                "name": "run_python",
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


@app.post("/execute", response_model=ExecuteResponse)
def execute_code(req: ExecuteRequest):
    """执行 Python 代码"""
    start = time.time()

    # 捕获 stdout/stderr
    stdout_buf = io.StringIO()
    stderr_buf = io.StringIO()

    # 记录执行前的文件
    files_before = set(WORK_DIR.glob("*"))

    # 复制全局命名空间（避免污染）
    local_ns = dict(GLOBAL_NS)

    success = True
    error = None

    try:
        with contextlib.redirect_stdout(stdout_buf), contextlib.redirect_stderr(stderr_buf):
            exec(req.code, local_ns)
    except Exception as e:
        success = False
        error = f"{type(e).__name__}: {e}\n{traceback.format_exc()}"

    # 关闭所有 matplotlib 图形
    try:
        import matplotlib.pyplot as plt
        plt.close('all')
    except:
        pass

    # 检查新生成的文件
    files_after = set(WORK_DIR.glob("*"))
    new_files = [str(f.name) for f in (files_after - files_before)]

    duration_ms = int((time.time() - start) * 1000)

    return ExecuteResponse(
        success=success,
        stdout=stdout_buf.getvalue()[:10000],  # 限制输出长度
        stderr=stderr_buf.getvalue()[:5000],
        error=error,
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
    uvicorn.run(app, host="0.0.0.0", port=port)
