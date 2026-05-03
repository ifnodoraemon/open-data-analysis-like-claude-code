import os
import requests
import pytest

BASE_URL = os.environ.get("PYTHON_EXECUTOR_BASE_URL", "http://localhost:8081")
PROXY_TOKEN = os.environ.get("PROXY_TOKEN", "test-token")


def _executor_available():
    try:
        resp = requests.get(f"{BASE_URL}/health", timeout=1)
        return resp.status_code == 200
    except requests.RequestException:
        return False


pytestmark = pytest.mark.skipif(
    not _executor_available(),
    reason="python executor integration service is not running",
)


def _headers():
    return {"X-Proxy-Token": PROXY_TOKEN}


def test_os_import_blocked():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "import os\nprint(os.getcwd())",
            "timeout": 5,
        },
        headers=_headers(),
    )
    data = resp.json()
    assert not data["success"]
    assert "not allowed" in data["error"].lower() or "import" in data["error"].lower()


def test_importlib_blocked():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "import importlib\nimportlib.import_module('os')",
            "timeout": 5,
        },
        headers=_headers(),
    )
    data = resp.json()
    assert not data["success"]


def test_ctypes_blocked():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "import ctypes\nctypes.CDLL('libc.so.6')",
            "timeout": 5,
        },
        headers=_headers(),
    )
    data = resp.json()
    assert not data["success"]


def test_dunder_subclasses_blocked():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "x = ().__class__.__bases__[0].__subclasses__()",
            "timeout": 5,
        },
        headers=_headers(),
    )
    data = resp.json()
    assert not data["success"]


def test_pathlib_blocked():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "from pathlib import Path\nprint(list(Path('/').glob('*')))",
            "timeout": 5,
        },
        headers=_headers(),
    )
    data = resp.json()
    assert not data["success"]


def test_globals_blocked():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "g = globals()\nprint(g)",
            "timeout": 5,
        },
        headers=_headers(),
    )
    data = resp.json()
    assert not data["success"]


def test_type_blocked():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "type('X', (object,), {})",
            "timeout": 5,
        },
        headers=_headers(),
    )
    data = resp.json()
    assert not data["success"]


def test_timeout():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "while True: pass",
            "timeout": 5,
        },
        headers=_headers(),
    )
    data = resp.json()
    assert not data["success"]
    assert "timed out" in data["error"].lower()


def test_timeout_max_cap():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "print('hello')",
            "timeout": 999999,
        },
        headers=_headers(),
    )
    data = resp.json()
    assert data["success"]


def test_happy_path():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "print('hello world')",
            "timeout": 5,
        },
        headers=_headers(),
    )
    data = resp.json()
    assert data["success"]
    assert "hello world" in data["stdout"]


def test_pandas_available():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "import pandas as pd\ndf = pd.DataFrame({'a': [1,2,3]})\ndf['b'] = df['a'] * 2\nprint(df.to_string())",
            "timeout": 10,
        },
        headers=_headers(),
    )
    data = resp.json()
    assert data["success"], f"pandas should work: {data.get('error', '')}"


def test_file_access_restricted():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "with open('/etc/passwd', 'r') as f:\n    print(f.read()[:50])",
            "timeout": 5,
        },
        headers=_headers(),
    )
    data = resp.json()
    assert not data["success"]


def test_proxy_token_required():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "print('hello')",
            "timeout": 5,
        },
    )
    assert resp.status_code in (403, 503)


def test_invalid_proxy_token():
    resp = requests.post(
        f"{BASE_URL}/execute",
        json={
            "code": "print('hello')",
            "timeout": 5,
        },
        headers={"X-Proxy-Token": "wrong-token"},
    )
    assert resp.status_code == 403


if __name__ == "__main__":
    for name, fn in sorted(globals().items()):
        if name.startswith("test_"):
            try:
                fn()
                print(f"PASS: {name}")
            except Exception as e:
                print(f"FAIL: {name}: {e}")
