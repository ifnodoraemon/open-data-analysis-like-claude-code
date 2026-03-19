#!/usr/bin/env python3
import argparse
import os
from pathlib import Path
import sys
import yaml

ROOT = Path(__file__).resolve().parents[1]
SCENARIO_ROOT = ROOT / 'samples' / 'coverage_scenarios'

BASE_CHECKS = [
    ('did_inspect_schema', '是否先查看了表结构 / schema / 字段信息？'),
    ('did_auto_map_fields', '该自动匹配字段名时，是否匹配成功？'),
    ('did_ask_user', '遇到真正歧义时，是否主动询问用户而不是硬猜？'),
    ('did_finalize_report', '明确要求生成图表/报告时，是否真正落地了最终报告？'),
    ('did_overclaim', '是否避免了在证据不足时给出强结论？'),
    ('did_join_correctly', '多表时是否正确处理了 join / grain / unit？'),
    ('did_state_limits', '是否明确写出了限制、缺口或假设？'),
]


def load_yaml(path: Path):
    with path.open('r', encoding='utf-8') as f:
        return yaml.safe_load(f)


def iter_scenarios(selected_id=None):
    for path in sorted(SCENARIO_ROOT.glob('*/scenario.yaml')):
        data = load_yaml(path)
        if selected_id and data.get('id') != selected_id:
            continue
        yield path.parent, data


def render_one(folder: Path, data: dict) -> str:
    expected = data.get('expected', {}) or {}
    files = data.get('files', []) or []
    lines = []
    lines.append(f"# {data.get('id', folder.name)}")
    lines.append('')
    lines.append(f"- 目录: `{folder.relative_to(ROOT)}`")
    lines.append(f"- 行业: `{data.get('industry', 'unknown')}`")
    lines.append(f"- 任务跨度: `{data.get('task_length', 'unknown')}`")
    lines.append(f"- 提问: {data.get('prompt', '')}")
    lines.append('- 上传文件:')
    for file_name in files:
        lines.append(f"  - `{folder.relative_to(ROOT)}/{file_name}`")
    lines.append('')
    lines.append('## 预期')
    lines.append(f"- coverage: `{expected.get('expected_coverage', 'unknown')}`")
    lines.append(f"- 应先 inspect schema: `{expected.get('should_inspect_schema', False)}`")
    lines.append(f"- 应自动映射字段: `{expected.get('should_auto_map', False)}`")
    lines.append(f"- 应主动询问用户: `{expected.get('should_ask_user', False)}`")
    if 'should_finalize_report' in expected:
        lines.append(f"- 应落地最终报告: `{expected.get('should_finalize_report', False)}`")
    req = expected.get('required_mentions', []) or []
    forbid = expected.get('forbidden_claims', []) or []
    req_tools = expected.get('required_tool_calls', []) or []
    req_codes = expected.get('required_tool_result_codes', []) or []
    if req:
        lines.append('- 必须出现的点:')
        for item in req:
            lines.append(f"  - {item}")
    if forbid:
        lines.append('- 不应出现的说法:')
        for item in forbid:
            lines.append(f"  - {item}")
    if req_tools:
        lines.append('- 必须调用的工具:')
        for item in req_tools:
            lines.append(f"  - {item}")
    if req_codes:
        lines.append('- 必须出现的工具结果码:')
        for item in req_codes:
            lines.append(f"  - {item}")
    lines.append('')
    lines.append('## 人工验收 Checklist')
    for key, label in BASE_CHECKS:
        expectation = expected.get(key)
        if expectation is None:
            lines.append(f"- [ ] {label}")
        else:
            lines.append(f"- [ ] {label} 期望=`{expectation}`")
    lines.append('')
    lines.append('## 记录')
    lines.append('- [ ] 最终报告是否回答了用户问题')
    lines.append('- [ ] 是否生成了合适的图表')
    lines.append('- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表')
    lines.append('- [ ] 是否需要补充新的测试场景')
    return '\n'.join(lines)


def main():
    parser = argparse.ArgumentParser(description='Render scenario checklist for manual evaluation.')
    parser.add_argument('--id', help='Only render one scenario id')
    parser.add_argument('--output', help='Output markdown file path')
    args = parser.parse_args()

    rendered = []
    for folder, data in iter_scenarios(args.id):
        rendered.append(render_one(folder, data))

    if not rendered:
        print('No scenarios found.', file=sys.stderr)
        sys.exit(1)

    doc = '\n\n---\n\n'.join(rendered) + '\n'
    if args.output:
        out_path = Path(args.output)
        if not out_path.is_absolute():
            out_path = ROOT / out_path
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(doc, encoding='utf-8')
    else:
        sys.stdout.write(doc)


if __name__ == '__main__':
    main()
