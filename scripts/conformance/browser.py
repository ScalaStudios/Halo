import base64
import html
import html.parser
import io
import json
import re
import sys
import time
import traceback
import urllib.parse

from PIL import Image, ImageDraw, ImageFont

from halo import HALO, SUITE, USER_EMAIL, USER_PASSWORD, Browser

suite = Browser()
browsers = {}
handled = set()
admin_group = json.load(open("out/config/admin.json"))["group_id"]
admin = Browser(open("out/config/admin-cookie").read().strip())


class Forms(html.parser.HTMLParser):
    def __init__(self):
        super().__init__()
        self.forms = []

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == "form":
            self.forms.append({"action": attrs.get("action", ""), "method": (attrs.get("method") or "get").upper(), "fields": {}})
        elif tag == "input" and self.forms and attrs.get("name"):
            self.forms[-1]["fields"][attrs["name"]] = attrs.get("value", "")


def log(test, message):
    print(f"[browser {test}] {message}", flush=True)


def placeholders(test):
    res = suite.request("GET", f"{SUITE}/api/log/{test}/images")
    if res.status != 200:
        return []
    return [entry["upload"] for entry in res.json() if entry.get("upload") and not entry.get("img")]


def capture(test, lines):
    pending = placeholders(test)
    if not pending:
        return
    canvas = Image.new("RGB", (1400, 48 + 24 * len(lines)), "white")
    draw = ImageDraw.Draw(canvas)
    font = ImageFont.load_default(16)
    for i, line in enumerate(lines):
        draw.text((16, 16 + 24 * i), line[:150], fill="black", font=font)
    png = io.BytesIO()
    canvas.save(png, "PNG")
    image = "data:image/png;base64," + base64.b64encode(png.getvalue()).decode()
    for name in pending:
        res = suite.request("POST", f"{SUITE}/api/log/{test}/images/{name}", image, {"Content-Type": "text/plain"})
        log(test, f"filled placeholder {name}: {res.status}")


def page_text(res):
    body = re.sub(r"<(script|style)[^>]*>.*?</\1>", " ", res.text, flags=re.S)
    return " ".join(html.unescape(re.sub(r"<[^>]+>", " ", body)).split())[:600]


def sign_in(test, b, url, trail):
    request_id = urllib.parse.parse_qs(urllib.parse.urlsplit(url).query).get("authRequest", [""])[0]
    if not request_id:
        return None
    if b.signed_in:
        res = b.request("POST", HALO + "/api/v1/auth/continue", {"authRequest": request_id})
        trail.append(f"POST /api/v1/auth/continue -> {res.status}")
        if res.status == 200 and res.json().get("redirect"):
            return urllib.parse.urljoin(HALO, res.json()["redirect"])
        capture(test, trail + ["Halo asked the person to sign in again"])
    assign(test, request_id)
    res = b.request("POST", HALO + "/api/v1/auth/password", {"email": USER_EMAIL, "password": USER_PASSWORD, "authRequest": request_id})
    trail.append(f"POST /api/v1/auth/password -> {res.status}")
    if res.status == 200 and res.json().get("redirect"):
        b.signed_in = True
        return urllib.parse.urljoin(HALO, res.json()["redirect"])
    log(test, f"sign-in failed: {res.status} {res.text[:300]}")
    return None


def assign(test, request_id):
    info = Browser().request("GET", f"{HALO}/api/v1/auth/request/{request_id}")
    name = info.json()["application"]["name"] if info.status == 200 else None
    apps = admin.api("GET", "/api/v1/applications")
    apps = apps.get("applications", apps) if isinstance(apps, dict) else apps
    for app in apps:
        if app.get("name") == name and admin_group not in (app.get("groupIds") or []):
            admin.api("PUT", f"/api/v1/applications/{app['id']}/groups", {"groupIds": (app.get("groupIds") or []) + [admin_group]})
            log(test, f"assigned the conformance group to the registered client {name}")


def consent(test, b, url, trail):
    request_id = urllib.parse.parse_qs(urllib.parse.urlsplit(url).query).get("consent", [""])[0]
    res = b.request("POST", HALO + "/api/v1/auth/consent", {"authRequest": request_id, "allow": True})
    trail.append(f"POST /api/v1/auth/consent -> {res.status}")
    if res.status == 200 and res.json().get("redirect"):
        return urllib.parse.urljoin(HALO, res.json()["redirect"])
    log(test, f"consent failed: {res.status} {res.text[:300]}")
    return None


def signed_out(test, b, url, trail):
    query = urllib.parse.parse_qs(urllib.parse.urlsplit(url).query)
    if query.get("confirm") == ["1"]:
        capture(test, trail + ["Halo asked the person to confirm signing out, and they chose Sign out"])
    res = b.request("POST", HALO + "/api/v1/auth/sign-out", {})
    trail.append(f"POST /api/v1/auth/sign-out -> {res.status}")
    b.signed_in = False
    after = query.get("next", [""])[0]
    if not after:
        trail.append("Halo showed the signed-out page with nowhere to return to")
        return None
    return HALO + "/oauth2/logout/return?" + urllib.parse.urlencode({"next": after})


def submit_fragment(test, b, res, fragment):
    match = re.search(r"xhr\.open\('POST',\s*\"([^\"]+)\"", res.text)
    if not match:
        return False
    sent = b.request("POST", match.group(1).replace("\\/", "/"), fragment, {"Content-Type": "text/plain"})
    log(test, f"submitted the callback to the suite: {sent.status}")
    return True


def visit(test, url, method):
    b = browsers.setdefault(test, Browser())
    b.signed_in = getattr(b, "signed_in", False)
    trail = []
    form = None
    for _ in range(20):
        fragment = ""
        if "#" in url:
            url, fragment = url.split("#", 1)
            fragment = "#" + fragment
        if form is not None:
            res = b.request("POST", url, form=form)
        else:
            res = b.request(method, url)
        trail.append(f"{'POST' if form is not None else method} {url[:120]} -> {res.status}")
        form, method = None, "GET"
        if url.startswith(SUITE):
            if not submit_fragment(test, b, res, fragment) and res.status >= 400:
                log(test, f"suite answered {res.status}: {res.text[:200]}")
            if res.location:
                url = res.location
                continue
            return
        if res.location:
            nxt = res.location
            path = urllib.parse.urlsplit(nxt).path
            if nxt.startswith(HALO) and path == "/sign-in" and "consent=" in nxt:
                nxt = consent(test, b, nxt, trail)
            elif nxt.startswith(HALO) and path == "/sign-in":
                nxt = sign_in(test, b, nxt, trail)
            elif nxt.startswith(HALO) and path == "/signed-out":
                nxt = signed_out(test, b, nxt, trail)
            if not nxt:
                capture(test, trail)
                return
            url = nxt
            continue
        parser = Forms()
        parser.feed(res.text)
        posts = [f for f in parser.forms if f["method"] == "POST" and f["action"].startswith("http")]
        if res.status == 200 and posts:
            url, form = html.unescape(posts[0]["action"]), posts[0]["fields"]
            continue
        trail.append(page_text(res))
        log(test, f"stopped at {url[:120]}: {res.status} {page_text(res)[:200]}")
        capture(test, trail)
        return


def main():
    while True:
        try:
            res = suite.request("GET", f"{SUITE}/api/runner/running")
            for test in res.json() if res.status == 200 else []:
                info = suite.request("GET", f"{SUITE}/api/runner/{test}")
                if info.status != 200:
                    continue
                for entry in (info.json().get("browser") or {}).get("urlsWithMethod") or []:
                    key = (test, entry["url"])
                    if key in handled:
                        continue
                    handled.add(key)
                    suite.request("POST", f"{SUITE}/api/runner/browser/{test}/visit?" + urllib.parse.urlencode({"url": entry["url"]}))
                    log(test, f"visiting {entry.get('method', 'GET')} {entry['url'][:160]}")
                    visit(test, entry["url"], entry.get("method") or "GET")
        except Exception:
            traceback.print_exc()
        time.sleep(1)


if __name__ == "__main__":
    sys.exit(main())
