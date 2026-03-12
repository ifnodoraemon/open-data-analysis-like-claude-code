FROM python:3.12-slim

# 安装中文字体（matplotlib 中文支持）
RUN apt-get update && apt-get install -y --no-install-recommends \
    fonts-wqy-zenhei \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY main.py .
RUN mkdir -p /app/workspace

EXPOSE 8081

CMD ["python", "main.py"]
