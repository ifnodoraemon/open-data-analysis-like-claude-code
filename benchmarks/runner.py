#!/usr/bin/env python3
import os
import sys
import time
import json
import glob
from pathlib import Path

# A minimal benchmark runner that aggregates prompt metrics from llm-trace logs.

LLM_DEBUG_DIR = Path("data/llm-debug")

def parse_trace_metrics():
    """Parse trace files to collect prompt size metrics."""
    metrics = {
        "request_json_bytes": 0,
        "input_item_count": 0,
        "avg_tool_arg_bytes": 0,
        "tool_call_count": 0,
        "final_report_phase_prompt_bytes": 0,
    }

    if not LLM_DEBUG_DIR.exists():
        print(f"Warning: {LLM_DEBUG_DIR} does not exist. Run the agent first.")
        return metrics

    # Find the most recent date dir
    date_dirs = sorted([d for d in LLM_DEBUG_DIR.iterdir() if d.is_dir()], reverse=True)
    if not date_dirs:
        return metrics
    
    recent_dir = date_dirs[0]
    total_tool_arg_bytes = 0

    trace_dirs = [d for d in recent_dir.iterdir() if d.is_dir()]
    for trace_dir in trace_dirs:
        req_file = trace_dir / "request.json"
        
        if req_file.exists():
            size = req_file.stat().st_size
            metrics["request_json_bytes"] += size
            
            try:
                with open(req_file, 'r', encoding='utf-8') as f:
                    req_data = json.load(f)
                    
                    messages = req_data.get("messages", [])
                    metrics["input_item_count"] += len(messages)
                    
                    # Track reporting phase size specifically
                    for msg in messages:
                        if "report_finalize" in json.dumps(msg):
                            metrics["final_report_phase_prompt_bytes"] += size
            except Exception as e:
                pass

        # Parse tool arguments specifically if tracked in index.jsonl
        # The agent logs event types in index.jsonl inside the date dir
    
    index_file = recent_dir / "index.jsonl"
    if index_file.exists():
        try:
            with open(index_file, 'r', encoding='utf-8') as f:
                for line in f:
                    try:
                        record = json.loads(line)
                        if record.get("event") == "tool.call":
                            payload = record.get("payload", {})
                            metrics["tool_call_count"] += 1
                            total_tool_arg_bytes += payload.get("arguments_bytes", 0)
                    except json.JSONDecodeError:
                        pass
        except Exception:
            pass

    if metrics["tool_call_count"] > 0:
        metrics["avg_tool_arg_bytes"] = total_tool_arg_bytes / metrics["tool_call_count"]

    return metrics

def run_benchmarks():
    print("=== Open Data Analysis Benchmark Runner ===")
    print("Scanning benchmarks/cases...")
    
    cases_dir = Path("benchmarks/cases")
    if not cases_dir.exists():
        print("No cases found. Create tests in benchmarks/cases/")
        return
        
    for case in cases_dir.iterdir():
        if case.is_dir():
            print(f"Running case: {case.name}")
            # Placeholder for actual API invocation
            print(f" (Placeholder: Submit workspace API and await completion...) \n")
            
    # Print metrics collected during this run
    metrics = parse_trace_metrics()
    print("\n=== LLM Trace Metrics Summary ===")
    print(f"Total Request JSON Bytes: {metrics['request_json_bytes']}")
    print(f"Total Input Items: {metrics['input_item_count']}")
    print(f"Average Tool Arg Bytes: {metrics['avg_tool_arg_bytes']:.2f}")
    if metrics["final_report_phase_prompt_bytes"] > 0:
        print(f"Final Report Phase Prompt Bytes: {metrics['final_report_phase_prompt_bytes']}")
    else:
        print("Final Report Phase Prompt Bytes: N/A (No report tools called)")

if __name__ == "__main__":
    run_benchmarks()
