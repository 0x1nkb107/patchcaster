# PatchCaster requirements

PatchCaster is a cloud native Go service that forwards GitLab (19.0 and later) merge request
webhook events into Mattermost channels, one thread per merge request.

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in BCP 14 [RFC 2119] [RFC 8174] when, and only when, they appear in all capitals, as shown here.

Requirements follow the SARA model declared in `../../sara.toml`: solution → use case → scenario →
system requirement → system architecture, with architecture decision records justifying
architecture choices. An absent `status` on an item means `active`.
