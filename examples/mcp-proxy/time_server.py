from datetime import datetime
from fastmcp import FastMCP

app = FastMCP("clock", stateless_http=True, json_response=True)

@app.tool("time/now")
def now() -> str:
    return datetime.now().isoformat()

if __name__ == "__main__":
    app.run("http", host="127.0.0.1", port=7777)
