import time
from urllib.error import URLError

from halo import HALO, SUITE, Browser

b = Browser()
for _ in range(120):
    try:
        if b.request("GET", HALO + "/.well-known/openid-configuration").status == 200 and b.request("GET", SUITE + "/api/runner/available").status == 200:
            break
    except (URLError, TimeoutError):
        pass
    time.sleep(2)
else:
    raise SystemExit("Halo or the conformance suite did not start")
