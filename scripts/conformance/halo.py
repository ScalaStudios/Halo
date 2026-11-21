import json
import os
import ssl
import urllib.error
import urllib.parse
import urllib.request
from http.cookiejar import CookieJar, Cookie

HALO = os.environ.get("HALO_URL", "https://halo.test")
SUITE = os.environ.get("CONFORMANCE_SERVER", "https://localhost.emobix.co.uk:8443/").rstrip("/")
HERE = os.path.dirname(os.path.abspath(__file__))
TLS = ssl.create_default_context(cafile=os.path.join(HERE, "out", "certs", "tls.crt"))
USER_EMAIL = "conformance@halo.test"
USER_PASSWORD = "Conformance-Suite-Password-28"
ALIAS = "halo"


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class Response:
    def __init__(self, status, headers, body, url):
        self.status = status
        self.headers = headers
        self.body = body
        self.url = url

    def json(self):
        return json.loads(self.body or b"null")

    @property
    def text(self):
        return self.body.decode("utf-8", "replace")

    @property
    def location(self):
        value = self.headers.get("Location")
        return urllib.parse.urljoin(self.url, value) if value else None


class Browser:
    def __init__(self, cookie=None):
        self.jar = CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.HTTPSHandler(context=TLS), urllib.request.HTTPCookieProcessor(self.jar), NoRedirect)
        if cookie:
            name, value = cookie.split("=", 1)
            self.jar.set_cookie(Cookie(0, name, value, None, False, "halo.test", False, False, "/", True, True, None, False, None, None, {}))

    def request(self, method, url, body=None, headers=None, form=None):
        headers = dict(headers or {})
        data = None
        if form is not None:
            data = urllib.parse.urlencode(form).encode()
            headers.setdefault("Content-Type", "application/x-www-form-urlencoded")
        elif body is not None and not isinstance(body, (bytes, str)):
            data = json.dumps(body).encode()
            headers.setdefault("Content-Type", "application/json")
        elif body is not None:
            data = body.encode() if isinstance(body, str) else body
        if url.startswith(HALO) and method != "GET":
            headers.setdefault("Origin", HALO)
        req = urllib.request.Request(url, data=data, method=method, headers=headers)
        try:
            with self.opener.open(req, timeout=30) as res:
                return Response(res.status, res.headers, res.read(), url)
        except urllib.error.HTTPError as err:
            return Response(err.code, err.headers, err.read(), url)

    def api(self, method, path, body=None, expect=(200, 201, 204)):
        res = self.request(method, HALO + path, body)
        if res.status not in expect:
            raise RuntimeError(f"{method} {path}: {res.status} {res.text}")
        return res.json() if res.body else None
