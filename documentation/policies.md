# Access policies

Access policies decide whether a sign-in may go ahead. Every sign-in asks the policy engine for a decision before Halo creates a session or sends the person back to an application: a passkey or code sign-in, a magic link, a federated sign-in, continuing an existing Halo session into an application, silent sign-in with `prompt=none`, and approving a device on the `halo login` confirmation page.

Security administrators manage policies, network zones, sign-in methods and device trust. Every administrator can read them and run simulations.

## How Halo decides

For each sign-in, Halo works out the context, then decides in this order:

1. If an administrator blocked the device, Halo blocks the sign-in.
2. If the sign-in method is turned off for the organization, Halo blocks the sign-in.
3. Otherwise it checks every enabled policy against the sign-in. Of the **enforced** policies that match, the strictest effect wins: **block**, then **require a passkey or security key**, then **allow**. When two matching policies have the same effect, the earlier one in the list is named as the reason.
4. When no enforced policy matches, Halo allows the sign-in.

An **allow** policy never overrides a stricter policy that also matches. To exempt people from a policy, add them, or a group of theirs, to that policy's exclusions.

**Require a passkey or security key** lets the sign-in through when the person used a passkey or security key, and refuses it with `ERR_STRONGER_AUTH_REQUIRED` otherwise. The person can sign in again with a phishing-resistant method. A Halo session started with an authenticator code cannot continue into an application that requires one.

Refused sign-ins appear in the sign-in log with the deciding policy and its reason, such as `Policy “Admins use passkeys”: Requires a passkey or security key for sign-ins by members of Administrators to Halo.`

## Write a policy

1. In the console, open **Policies** and choose **Create policy**.
2. Name the policy and describe why it exists.
3. Choose who it applies to: everyone, or chosen groups and people. Optionally exclude groups and people; an exclusion always wins.
4. Choose which applications it covers: all of them, or chosen ones. **Halo** in the list means the console and the account portal themselves.
5. Add conditions. A policy matches only when every condition holds:

   | Condition | Matches when |
   | --- | --- |
   | Network | **Inside selected networks**: the client IP address is in one of the chosen network zones. **Outside selected networks**: it is in none of them. **Any network** ignores it. |
   | Device | **Trusted devices only**, or **Devices that aren't trusted** (unknown or blocked). **Any device** ignores it. |
   | Risk | The sign-in's risk level is one of the chosen levels: none, low, medium or high. No levels selected matches every level. |
   | Sign-in method | The person signs in with one of the chosen methods. None selected matches every method. |

6. Choose the effect: **Allow**, **Require a passkey or security key**, or **Block**.
7. Choose the mode, **Enforced** or **Report-only**, and choose **Save policy**.

Turning a policy off makes Halo skip it until you turn it on again. The policy list keeps a saved order, which you change with the move up and move down buttons. The order only decides which policy is named when several with the same effect match.

### Examples

| Goal | Applies to | Covers | Conditions | Effect |
| --- | --- | --- | --- | --- |
| Administrators always use passkeys | Group `Administrators` | Halo | | Require a passkey or security key |
| Block sign-ins from a risky network | Everyone | All applications | Inside `Tor exit nodes` | Block |
| Keep high-risk sign-ins out of finance tools | Everyone | `Billing` | Risk: high | Block |
| No magic links for production access | Group `Production` | All applications | Sign-in method: magic link | Block |

## Report-only mode

A policy in report-only mode is evaluated on every sign-in like an enforced one, but it never changes the outcome. Halo records what it would have done, and the policy list shows, for the past 24 hours, how many sign-ins each policy matched and how many it would have stopped. Start new policies in report-only mode, watch the numbers and the sign-in log for a few days, then switch them to enforce.

Halo keeps the record of each evaluation for 30 days.

## Network zones

A network zone is a named list of IP addresses and ranges, such as your office or a VPN.

1. In the console, open **Policies**, go to **Networks**, and choose **Add network**.
2. Name it, and choose whether it is **trusted** or **risky**.
3. Enter IPv4 and IPv6 addresses or CIDR ranges, such as `203.0.113.0/24` or `2001:db8::/48`, up to 500 per zone. Halo stores single addresses as `/32` or `/128` ranges.

Policies use zones in their network condition. A **risky** zone also raises the risk of every sign-in from it to high, whether or not a policy names the zone.

A zone that a policy uses cannot be deleted; remove it from the policy first.

Halo matches the client's IP address. Behind a reverse proxy, set [`HALO_TRUSTED_PROXIES`](configuration.md#halo_trusted_proxies), or Halo sees the proxy's address for everyone.

## Devices

Halo gives every browser a `halo_device` cookie with a random value that lasts 2 years. The pair of a person and that cookie is a **device**. Halo records a device at each sign-in attempt, with its browser, operating system, last IP address and number of successful sign-ins. The cookie does not authenticate anyone.

Each device has a trust level:

| Trust | Effect |
| --- | --- |
| Unknown | The default. Policies with the device condition **Devices that aren't trusted** match it. |
| Trusted | Policies with the device condition **Trusted devices only** match it. |
| Blocked | Every sign-in from it is refused. |

Security administrators set the trust in the console under **Devices**. People see their own devices at `GET /api/v1/me/devices`, with `current` marking the browser they are using.

Clearing cookies or switching browsers creates a new, unknown device.

## Sign-in risk

Halo assesses every sign-in with these signals:

| Signal | Level | When |
| --- | --- | --- |
| Risky network | High | The IP address is in a risky network zone. |
| Repeated failed sign-ins | High | The account, or the email address typed, had 5 or more failed sign-ins in the past 15 minutes. |
| New device | Medium | The browser has never been used by this person, and the person already signed in from other devices. |
| New network | Low | The person has signed in successfully before, but never from this network: the same /24 for IPv4 or /48 for IPv6. |

The sign-in's risk is the highest level among its signals, or **none**. The sign-in log shows it on every event, and policies can match on it.

Medium and high signals also create **risk events**, listed in the console under **Risk events** with the person, IP address, device and detail. A security administrator marks each one **resolved** after acting on it, or **dismissed** when it was expected. Resolving or dismissing an event changes nothing about the person's access; it records that someone looked.

## Allowed sign-in methods

The console's **Methods** page turns sign-in methods on and off for the whole organization: passkeys, security keys, authenticator apps, magic links and recovery codes. It shows how many active people have each method, and how many depend on it as their only enabled method.

- A method that is off disappears from the sign-in page, and sign-ins with it are blocked with the reason "Sign-in with … is turned off for this organization."
- At least one of passkeys, security keys, authenticator apps and magic links must stay on.
- While magic links are off, Halo sends none, and answers a request for one exactly as it does for an unknown address.
- Sign-in through an identity provider is turned on and off per provider under **Identity providers**; see [Federation](federation.md).

To allow a method for some people and not others, keep it on and write a policy with a sign-in method condition, such as the magic link example above.

## Simulate a sign-in

The simulator shows what the policies would decide without anyone signing in.

1. In the console, open **Policies** and choose **Simulate sign-in**.
2. Pick the person, the application (or Halo), an IP address, the sign-in method and the device trust.
3. Halo shows the decision, the risk level with each signal, and for every enabled policy whether it matched and why or why not.

The simulation records nothing. It uses the person's real sign-in history for the failed sign-in and new network signals, and does not raise the new device signal.

## API

The [API](api.md) covers policies at `/api/v1/policies` (with `/api/v1/policies/order` and `/api/v1/policies/simulate`), network zones at `/api/v1/network-zones`, methods at `/api/v1/methods`, devices at `/api/v1/devices` and risk events at `/api/v1/risk-events`. `GET /api/v1/auth/methods` lists the enabled methods without authentication; the sign-in page uses it.
