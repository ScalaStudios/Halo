import json
import os
import sys

from halo import ALIAS, HALO, HERE, SUITE, USER_EMAIL, USER_PASSWORD, Browser

base = f"{SUITE}/test/a/{ALIAS}"
admin = Browser(os.environ["HALO_ADMIN_COOKIE"])

admin.api("PUT", "/api/v1/methods/password", {"enabled": True})
admin.api("PATCH", "/api/v1/settings", {"clientRegistration": "open", "passwordSecondFactor": False})
group = admin.api("POST", "/api/v1/groups", {"name": "Conformance", "kind": "assigned"})
group_id = group["id"]
created = admin.api("POST", "/api/v1/users", {"email": USER_EMAIL, "name": "Conformance User", "groupIds": [group_id]})
user_id = created["user"]["id"]
admin.api("PUT", f"/api/v1/users/{user_id}/password", {"password": USER_PASSWORD + "-temporary"})
first = Browser().request("POST", HALO + "/api/v1/auth/password", {"email": USER_EMAIL, "password": USER_PASSWORD + "-temporary", "newPassword": USER_PASSWORD})
if first.status not in (200, 201, 204):
    sys.exit(f"first password sign-in: {first.status} {first.text}")


def application(name, backchannel=False):
    app = admin.api("POST", "/api/v1/applications", {
        "name": name,
        "protocol": "oidc",
        "type": "web",
        "redirectUris": [f"{base}/callback", f"{base}/callback?dummy1=lorem&dummy2=ipsum"],
        "postLogoutUris": [f"{base}/post_logout_redirect"],
        "scopes": ["openid", "profile", "email", "offline_access"],
        "groupIds": [group_id],
    })
    if backchannel:
        admin.api("PATCH", f"/api/v1/applications/{app['application']['id']}", {"backchannelLogoutUri": f"{base}/backchannel_logout", "backchannelLogoutSessionRequired": True})
    return {"client_id": app["application"]["clientId"], "client_secret": app["clientSecret"]}


server = {"discoveryUrl": HALO + "/.well-known/openid-configuration"}
static = {"alias": ALIAS, "description": "Halo", "server": server, "client": application("Conformance client"), "client2": application("Conformance second client"), "client_secret_post": application("Conformance post client")}
backchannel = {"alias": ALIAS, "description": "Halo", "server": server, "client": application("Conformance back-channel client", True), "client2": application("Conformance second back-channel client", True)}
dynamic = {"alias": ALIAS, "description": "Halo", "server": server}

os.makedirs(os.path.join(HERE, "out", "config"), exist_ok=True)
for name, config in {"static": static, "backchannel": backchannel, "dynamic": dynamic}.items():
    with open(os.path.join(HERE, "out", "config", name + ".json"), "w") as f:
        json.dump(config, f, indent=2)
with open(os.path.join(HERE, "out", "config", "admin-cookie"), "w") as f:
    f.write(os.environ["HALO_ADMIN_COOKIE"])
with open(os.path.join(HERE, "out", "config", "admin.json"), "w") as f:
    json.dump({"group_id": group_id}, f)
print("seeded conformance fixtures")
