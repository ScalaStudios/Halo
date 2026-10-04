# Access governance

Halo grants access through group membership. Governance decides how people get into groups and how long they stay there:

- **Access packages** bundle groups that people request themselves, with approvers and an optional time limit.
- **Access reviews** ask reviewers to confirm, once in a while, that each member of a group still needs to be there.
- **Lifecycle rules** change group memberships, sessions and account status automatically when someone joins, moves or leaves.

All three work on **assigned** groups. Members of rule-based groups come from their rule, so change the rule or people's profiles instead.

User administrators manage governance. Every administrator can read it, and every decision lands in the audit log.

## Access packages

An access package is a named set of assigned groups that people can request in the account portal.

### Create a package

1. In the console, open **Access packages** and choose **Create access package**.
2. Enter a name, such as `Production access`, and a description that tells people what they get.
3. Pick one or more assigned groups. An approved request adds the requester to all of them.
4. Pick the approvers. Without approvers, only a global administrator can decide requests.
5. Optionally set a **Maximum duration** between 1 and 365 days. People then have to choose a duration up to that limit, and access ends automatically when it runs out. Without a maximum, people may ask for access until it is revoked.
6. Optionally require a justification. People must then explain why they need access, and approvers read it before deciding.
7. Choose **Save package**.

The package's owner is informational: it names the person responsible for it and gives them no extra rights.

To stop new requests, archive the package. Halo refuses to delete a package that has request history, so the history stays readable; a package without requests can be deleted.

### Request access

1. In the account portal, open **Access** and choose **Request access**.
2. Pick the package, the duration if the package has a maximum, and a justification.
3. Halo emails every active approver other than the requester, with a link to the account portal, or to the console for approvers who hold an administrator role. When the package has no active approver besides the requester, it emails the active global administrators instead.

A person can have one pending request per package. They can cancel it from the same page until someone decides it.

### Approve or deny

An approver opens the request from the email, from **Access** in the account portal, or from **Requests** in the console, and approves or denies it with an optional note.

- Nobody can decide their own request, global administrators included; Halo answers `ERR_SELF_APPROVAL` and names who else can. A global administrator can decide every other request.
- Approving adds the requester to each of the package's groups and starts the clock on the duration. Halo emails the requester the decision, the groups and the end time.
- Applications see the new groups the next time the person signs in to them, because they receive the `groups` claim at sign-in.

### When access ends

A grant ends when its duration runs out or when a user administrator revokes it in the console. Halo checks for expired grants every minute.

When a grant ends, Halo removes the person from the groups that the grant added. It leaves a membership in place when:

- the person was already a member before the grant, or
- another active grant of theirs covers the same group. That grant then takes over the membership and removes it when it ends.

Each expiry and revocation writes an audit event that lists the memberships removed.

## Access reviews

An access review asks reviewers to decide, for every member of an assigned group, whether they keep access.

### Start a review

1. In the console, open **Access reviews** and choose **Start an access review**.
2. Name the review, such as `Production access · Q4`.
3. Pick the assigned group. It needs at least one member. Every member who is not deprovisioned becomes an item of the review.
4. Pick one or more reviewers. Reviewers do not need an administrator role.
5. Set a due date in the future and within one year.
6. Choose whether to remove people automatically. With it on, completing the review removes everyone marked for removal from the group. With it off, completing the review only records the decisions.

### Decide and complete

Reviewers find their open reviews in the account portal under **Access**, which links reviewers who hold an administrator role to the console instead. For each member they choose **keep** or **remove**, with an optional note. They can change a decision until the review is completed. A global administrator can also record decisions.

A reviewer or a user administrator completes the review. On completion, each item gets an outcome:

| Decision | Automatic removal on | Automatic removal off |
| --- | --- | --- |
| Keep | Kept | Kept |
| Remove | Removed from the group | Marked for removal but left in place |
| No decision | Keeps access | Keeps access |

After the due date, a review that is not complete becomes **overdue**, and Halo writes an audit event. Reviewers can still decide and complete an overdue review.

Who can see a review: its reviewers, and anyone with an administrator role.

## Lifecycle rules

A lifecycle rule runs actions when someone joins, moves or leaves.

| Trigger | When it fires |
| --- | --- |
| Joiner | A person's account is created: invited in the console, provisioned over SCIM, or created on first sign-in with an identity provider. |
| Mover | The department, title or location changes. |
| Leaver | The status becomes suspended or deprovisioned. |

A rule has:

- a **condition** in the [group rule syntax](concepts.md#rule-syntax), such as `user.department == "Engineering"`. The person's current profile must match it. Leave it empty to match everyone. Rules only ever match people; service accounts never trigger them.
- between 1 and 10 **actions**: **Add to group** and **Remove from group** for assigned groups, **Revoke sessions**, which ends every Halo session of the person, and **Suspend account**, which also ends their sessions.

### Create a rule

1. In the console, open **Lifecycle** and choose **Create lifecycle rule**.
2. Name it, pick the trigger, and enter the condition.
3. Add the actions in the order they should run.
4. Turn the rule on and choose **Save rule**.

To pause a rule and keep its run history, turn it off. To remove it, choose **Delete rule** in its row menu. Deleting also removes its runs from the run history; their `lifecycle.run` audit events stay. Halo refuses to delete a group while a lifecycle rule adds people to it or removes them from it.

### How rules run

Every minute, Halo compares each account with the state it last processed and finds the joiners, movers and leavers since then. For each change, it runs every enabled rule with that trigger whose condition matches, and records a run with each step and its result. Several changes to one person within the same minute count as one change.

A run **fails** when one of its steps cannot be done, for example when the group no longer exists or is rule-based, or when the rule would suspend the last active global administrator. The other steps still run. The console shows the run history under **Lifecycle**, and each run also writes a `lifecycle.run` audit event.

Example: a joiner rule with the condition `user.department == "Engineering"` and the action **Add to group** `Engineering tools` gives new engineers their tools on their first day. A leaver rule without a condition and the action **Remove from group** `Engineering tools` takes them away when the account is suspended or deprovisioned.

## API

Every governance action is available in the [API](api.md): `/api/v1/access-packages`, `/api/v1/access-requests`, `/api/v1/access-reviews`, `/api/v1/lifecycle/rules` and `/api/v1/lifecycle/runs` for administrators, and `/api/v1/me/access-packages`, `/api/v1/me/access-requests`, `/api/v1/me/access-grants`, `/api/v1/me/approvals` and `/api/v1/me/reviews` for people in the account portal.
