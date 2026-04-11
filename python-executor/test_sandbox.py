import requests
import time

BASE_URL = "http://localhost:8081"

def test_os_import_blocked():
    resp = requests.post(f"{BASE_URL}/execute", json={
        "code": "import os\nprint(os.getcwd())",
        "timeout": 5
    })
    data = resp.json()
    assert not data["success"], f"Expected failure, got success"
    assert "not allowed" in data["error"].lower() or "import" in data["error"].lower()

def test_importlib_blocked():
    resp = requests.post(f"{BASE_URL}/execute", json={
        "code": "import importlib\nimportlib.import_module('os')",
        "timeout": 5
    })
    data = resp.json()
    assert not data["success"], f"importlib should be blocked"

def test_ctypes_blocked():
    resp = requests.post(f"{BASE_URL}/execute", json={
        "code": "import ctypes\nctypes.CDLL('libc.so.6')",
        "timeout": 5
    })
    data = resp.json()
    assert not data["success"], f"ctypes should be blocked"

def test_dunder_subclasses_blocked():
    resp = requests.post(f"{BASE_URL}/execute", json={
        "code": "x = ().__class__.__bases__[0].__subclasses__()",
        "timeout": 5
    })
    data = resp.json()
    assert not data["success"], f"__subclasses__ access should be blocked"

def test_pathlib_blocked():
    resp = requests.post(f"{BASE_URL}/execute", json={
        "code": "from pathlib import Path\nprint(list(Path('/').glob('*')))",
        "timeout": 5
    })
    data = resp.json()
    assert not data["success"], f"pathlib should be blocked"

def test_timeout():
    resp = requests.post(f"{BASE_URL}/execute", json={
        "code": "while True: pass",
        "timeout": 2
    })
    data = resp.json()
    assert not data["success"]
    assert "timed out" in data["error"].lower()

def test_timeout_max_cap():
    resp = requests.post(f"{BASE_URL}/execute", json={
        "code": "print('hello')",
        "timeout": 999999
    })
    data = resp.json()
    assert data["success"]

def test_happy_path():
    resp = requests.post(f"{BASE_URL}/execute", json={
        "code": "print('hello world')",
        "timeout": 5
    })
    data = resp.json()
    assert data["success"]
    assert "hello world" in data["stdout"]

def test_pandas_available():
    resp = requests.post(f"{BASE_URL}/execute", json={
        "code": "import pandas as pd\ndf = pd.DataFrame({'a': [1,2,3]})\ndf['b'] = df['a'] * 2\nprint(df.to_string())",
        "timeout": 10
    })
    data = resp.json()
    assert data["success"], f"pandas should work: {data.get('error', '')}"

def test_file_access_restricted():
    resp = requests.post(f"{BASE_URL}/execute", json={
        "code": "with open('/etc/passwd', 'r') as f:\n    print(f.read()[:50])",
        "timeout": 5
    })
    data = resp.json()
    assert not data["success"], f"File access outside sandbox should be blocked"

if __name__ == "__main__":
    for name, fn in list(globals().items()):
        if name.startswith("test_"):
            try:
                fn()
                print(f"PASS: {name}")
            except Exception as e:
                print(f"FAIL: {name}: {e}")
