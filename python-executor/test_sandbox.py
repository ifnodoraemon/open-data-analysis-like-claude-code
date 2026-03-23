import requests
import time

resp = requests.post("http://localhost:8082/execute", json={
    "code": "import os\nprint(os.getcwd())",
    "timeout": 5
})
print("OS import test:", resp.json())

resp2 = requests.post("http://localhost:8082/execute", json={
    "code": "import builtins\nprint('hello')",
    "timeout": 5
})
print("Builtins import test:", resp2.json())

resp3 = requests.post("http://localhost:8082/execute", json={
    "code": "import time\ntime.sleep(6)",
    "timeout": 2
})
print("Timeout test:", resp3.json())

resp4 = requests.post("http://localhost:8082/execute", json={
    "code": "print('hello world')",
    "timeout": 5
})
print("Success test:", resp4.json())

