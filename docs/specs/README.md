# PatchCaster requirements

PatchCaster is a cloud native Go service that forwards GitLab (19.0 and later) merge request
webhook events into Mattermost channels, one thread per merge request.

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in BCP 14 [RFC 2119] [RFC 8174] when, and only when, they appear in all capitals, as shown here.

Requirements follow the SARA model declared in `../../sara.toml`: solution → use case → scenario →
system requirement → system architecture, with architecture decision records justifying
architecture choices. An absent `status` on an item means `active`.

Each item lives in the directory of its type, in a file named after its id and a short lowercase slug, for example `adrs/ADR-001-sqlite-state.md`. A directory listing then sorts by number.

Any item may carry `status: proposed`. A proposed item describes planned behaviour that is not built yet, and it is not normative until the owner accepts it. Until then, the active items it would change stay in force, and the proposed item names them. Accepting a proposal removes its `status` line, or sets an ADR's status to `accepted`, and amends or retires the items it replaces. Proposals are accepted increment by increment, in the pull request that implements them, and SOL-001 lists the increments.

An item that spans several increments is accepted in part with the first increment that builds part of it. A note below its title names the parts still planned and the increment that brings each, and SOL-001 lists them. Those parts are not normative until their increment is accepted, and the item's links to proposed items lead to them.

An item that a newer one replaces carries `status: retired` and a note that names its successor. It stays for history and traceability, and it is not normative. A replaced ADR is `superseded` instead, as SARA defines it, and the ADR that replaces it names it under `supersedes`.

## Roles and parties

PatchCaster knows three roles. Text that people read uses the role names, and code uses the code names.

| Role | Responsibility | Code name |
| --- | --- | --- |
| Instance Admin | Instance-wide configuration and access | administrator |
| Routing Manager | Routing configuration within assigned teams | manager |
| Regular User | Personal identity links and tokens | regular |

An Instance Admin holds the role through a flag on the person. A Routing Manager is a person whom an Instance Admin named as a manager of at least one team. Everyone else who logs in is a Regular User. Each role may also do what the roles below it may.

Other parties hold no role in PatchCaster:

- The **infrastructure team** deploys and monitors an instance (UC-003). It acts through the command line and the database.
- **GitLab administrators** and **Mattermost administrators** administer the platforms that PatchCaster connects to.
