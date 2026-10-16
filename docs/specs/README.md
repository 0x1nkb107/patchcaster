# PatchCaster requirements

PatchCaster is a cloud native Go service that forwards GitLab (19.0 and later) merge request
webhook events into Mattermost channels, one thread per merge request.

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in BCP 14 [RFC 2119] [RFC 8174] when, and only when, they appear in all capitals, as shown here.

Requirements follow the SARA model declared in `../../sara.toml`: solution → use case → scenario →
system requirement → system architecture, with architecture decision records justifying
architecture choices. An absent `status` on an item means `active`.

Each item lives in the directory of its type, in a file named after its id and a short lowercase slug, for example `adrs/ADR-001-sqlite-state.md`. A directory listing then sorts by number.

Any item may carry `status: proposed`. A proposed item describes planned behaviour that is not built yet, and it is not normative until the owner accepts it. Until then, the active items it would change stay in force, and the proposed item names them. Accepting a proposal removes its `status` line, or sets an ADR's status to `accepted`, and amends or retires the items it replaces. Proposals are accepted increment by increment, in the pull request that implements them, and SOL-001 lists the increments.
