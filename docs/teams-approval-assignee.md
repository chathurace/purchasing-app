# Teams & approval assignees

As-built reference for the Teams settings section and the assignee on the
legal/security recommendation approval cards. Migrations `038` (teams) and `039`
(assignee).

## Teams

A **team** (`teams` table) is a named group — seeded fixed set **Legal**,
**Security**, **Procurement**. Legal/Security back the approval-card assignees
(below); Procurement backs procurement-role membership and the team-email
notifications. A team has:

- `key` — stable id used in URLs (`legal` / `security` / `procurement`).
- `name` — display label.
- `member_role` — the role that designates membership. **Fixed / display-only**:
  the approval-card actor logic is keyed on this role, so it is shown but not
  editable. (There is no separate membership table — a user is a member **iff**
  they hold `member_role`.)
- `team_email` — a shared address CC'd on the team's assignee notifications.

**Membership is a role.** Adding a member grants `member_role`; removing revokes
it (reusing `EnsureUserHasRole` / `RemoveUserRole`, the same grants surfaced on the
Users page). So a Legal team member is exactly a user with the `legal` role.

**Admin members.** A team may also have an **admin-role variant** that counts as
part of the team — currently only **Procurement**, whose admins hold
`procurement_admin`. These are surfaced separately as `admin_members` (derived, like
`members`): `Team.Members` stays scoped to plain `member_role` holders (so the base
membership — and everything keyed on it, e.g. the assignee pool in `AssignmentCard`
— is unchanged), while `admin_members` lists `procurement_admin` holders. Admins are
granted/revoked from the **Users** page, not the team card. `adminRoleForMemberRole`
in `repository/teams.go` maps `procurement → procurement_admin`; other teams have
none.

### Managed from Settings

The **Teams** section on the Settings page (`components/TeamsSection.tsx`) renders
one card per team: the member-role badge, the editable team email, the member list,
and an "add member" dropdown of active users. Base members are removable; the
Procurement team's `admin_members` are listed with an **admin** badge and no Remove
(a note points to the Users page). Access mirrors the rest of Settings —
**procurement_admin / admin** (`middleware.HasTeamAdmin`).

### API

| Method | Path | Access |
| --- | --- | --- |
| GET | `/api/v1/teams` | any authenticated (assignee dropdown needs membership) |
| PUT | `/api/v1/teams/{key}/email` | procurement_admin / admin |
| POST | `/api/v1/teams/{key}/members` `{user_id}` | procurement_admin / admin |
| DELETE | `/api/v1/teams/{key}/members/{userID}` | procurement_admin / admin |

Member add/remove records the existing `grant_role` / `revoke_role` audit events
(detail `team:<key>`).

## Assignee on legal/security approval cards

Each legal/security card on a PR's procurement recommendation now carries an
**assignee** (`pr_recommendation_approvals.assignee_id`, `ON DELETE SET NULL`). The
budget card has none.

**Permission split** (the card was previously all-or-nothing on the matching
role). Computed per-caller in `attachRecActionable` and serialized as three flags
on each card (`can_comment` / `can_approve` / `can_assign`):

- **Comment** — any member of the card's team (legal → `legal` role, security →
  `security` role), as before. (`canActOnRecType`)
- **Approve** — **only the card's assignee** may toggle approve/revert.
  (`canApproveRecType`) The budget card has no assignee — any qualified budget approver of the PR's
  budget unit (or admin) may approve; see `docs/budget-units.md`.
- **Assign** — any member of the card's team, **or** any procurement user
  (procurement / procurement_admin / admin). (`canAssignRecType`)

So a legal team member who wants to approve first assigns the card to themselves,
then approves. The server re-checks each predicate on every write — the flags are
only for showing/hiding controls.

The assignee must be an active member of the card's team (validated on write).

### Notifications (SMTP)

Setting an assignee opens a **notify?** prompt (a checkbox in the confirm dialog);
when checked, the assignee is emailed a link to the PR. A **Send reminder** button
next to the assignee re-sends it. Both **CC the team email**.

Delivery uses the existing `internal/email` SMTP mailer configured under `email:`
in `config.yaml` (`smtp_host` / `smtp_port` / `username` / `password` /
`from_address`, plus `app_base_url` for the link). When email is disabled (the dev
default) the send is logged, not sent. The `Mailer` interface gained `SendCC` for
the CC recipients. Sends are best-effort — a failure never blocks the action.

### API

| Method | Path | Access |
| --- | --- | --- |
| PUT | `.../recommendation/approvals/{type}/assignee` `{assignee_id, notify}` | team member or procurement |
| POST | `.../recommendation/approvals/{type}/assignee/remind` | team member or procurement |

`{type}` must be `legal` or `security`. Setting/clearing the assignee records a
`assign_rec_legal` / `assign_rec_security` process event (qualifier
`assign` / `unassign`).
