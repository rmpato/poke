import json, http.server, socketserver, time, itertools
orders = itertools.cycle([("pending", 120), ("completed", 120)])
class H(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    def send_json(self, code, obj, extra=None, delay=0):
        if delay: time.sleep(delay)
        body = json.dumps(obj, indent=2).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("X-Request-Id", "8f31c0de5512")
        self.send_header("Cache-Control", "no-store")
        for k, v in (extra or {}).items(): self.send_header(k, v)
        self.end_headers(); self.wfile.write(body)
    def do_GET(self):
        p = self.path
        if p.startswith("/users/42"):
            return self.send_json(200, {"id":42,"name":"Pato","email":"pato@example.com",
                "active":True,"tier":"gold","roles":["admin","billing"],
                "profile":{"city":"Madrid","timezone":"Europe/Madrid","avatar":None},
                "created_at":"2026-01-14T09:21:33Z"}, delay=0.018)
        if p.startswith("/users"):
            return self.send_json(200, {"users":[
                {"id":42,"name":"Pato","active":True},
                {"id":43,"name":"Ada","active":True},
                {"id":44,"name":"Grace","active":False}],"total":3,"page":1}, delay=0.012)
        if p.startswith("/orders/9021"):
            st, amt = next(orders)
            return self.send_json(200, {"id":9021,"status":st,"amount":amt,
                "currency":"EUR","items":[{"sku":"A-1","qty":2}],
                "customer":{"id":42,"name":"Pato"}}, delay=0.02)
        if p.startswith("/billing"):
            return self.send_json(403, {"error":"forbidden","message":"missing scope: billing.read"}, delay=0.009)
        return self.send_json(404, {"error":"not_found"}, delay=0.005)
    def do_POST(self):
        n = int(self.headers.get("Content-Length") or 0)
        payload = json.loads(self.rfile.read(n) or b"{}")
        if self.path.startswith("/login"):
            return self.send_json(201, {"token":"eyJhbGciOiJIUzI1NiJ9.demo","expires_in":3600}, delay=0.03)
        return self.send_json(201, {"id":45,"name":payload.get("name","?"),"active":True}, delay=0.022)
    def do_DELETE(self):
        self.send_response(204); self.send_header("Content-Length","0"); self.end_headers()
    def log_message(self, *a): pass
socketserver.TCPServer.allow_reuse_address = True
with socketserver.ThreadingTCPServer(("127.0.0.1", 8080), H) as s: s.serve_forever()
