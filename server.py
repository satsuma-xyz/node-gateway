import http.server
import json
import socketserver
import sys
import time

PORT = int(sys.argv[1])

METHOD_PREFIX = "eth_"

class CustomHandler(http.server.SimpleHTTPRequestHandler):
    
    def do_GET(self):
        # Always respond with 200 OK for GET requests, regardless of the path
        self.send_response(200)
        self.send_header("Content-type", "text/html")
        self.end_headers()
        response = "<html><body><h1>GET request received</h1></body></html>"
        self.wfile.write(response.encode('utf-8'))
    
    def do_POST(self):
        # Read the content length of the incoming POST data
        # noinspection PyTypeChecker
        content_length = int(self.headers['Content-Length'])
        post_data = self.rfile.read(content_length)  # Read the incoming POST data

        m = json.loads(post_data.decode('utf-8'))["method"]
        try:
            if not m.startswith(METHOD_PREFIX):
                raise ValueError
            suffix = m[len(METHOD_PREFIX):]
            c = int(suffix)
        except ValueError:
            c = 200

        secs = 0
        if c >= 1000:
            # Interpret the suffix as the response delay rather than the HTTP code.
            secs = c / 1000
            c = 200
        time.sleep(secs)

        if m == "eth_slowMethod":
            time.sleep(0.25)  # This would fail any other method, since the top-level latency threshold is 100ms.

        # Respond with the appropriate HTTP code.
        self.send_response(c)
        self.send_header("Content-type", "text/html")
        self.end_headers()

        # Log the received POST data
        print("Received POST data:", post_data.decode('utf-8'))

        # Respond to the POST request
        response = f"<html><body><h1>POST request received: port={PORT}, method={m}, secs={secs}</h1></body></html>"
        self.wfile.write(response.encode('utf-8'))

# Create server and bind to port 4444
# noinspection PyTypeChecker
with socketserver.TCPServer(("", PORT), CustomHandler) as httpd:
    print(f"Serving on port {PORT}")
    httpd.serve_forever()
