-- +goose Up
-- +goose StatementBegin
-- Teams scope what operators may route (ADR-034). Administrators name a
-- team's operators and assign it sources and sinks.
CREATE TABLE teams (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    version    BIGINT NOT NULL DEFAULT 1
);

CREATE TABLE team_operators (
    team_id   BIGINT NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    person_id BIGINT NOT NULL REFERENCES persons (id) ON DELETE CASCADE,
    PRIMARY KEY (team_id, person_id)
);
CREATE INDEX team_operators_person_idx ON team_operators (person_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- A source selects the repositories of one forge connection by scope: the
-- whole instance, a group or organization, or one repository (SYSREQ-044).
-- A connection with sources cannot be removed.
CREATE TABLE sources (
    id            BIGSERIAL PRIMARY KEY,
    connection_id BIGINT NOT NULL REFERENCES connections (id),
    name          TEXT NOT NULL UNIQUE,
    scope_kind    TEXT NOT NULL CHECK (scope_kind IN ('instance', 'group', 'repository')),
    scope_path    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    version       BIGINT NOT NULL DEFAULT 1,
    UNIQUE (connection_id, scope_kind, scope_path)
);

-- A sink names one channel or room of a chat connection (SYSREQ-045). The
-- channel name is how the platform named it at the last check.
CREATE TABLE sinks (
    id            BIGSERIAL PRIMARY KEY,
    connection_id BIGINT NOT NULL REFERENCES connections (id),
    name          TEXT NOT NULL UNIQUE,
    channel_id    TEXT NOT NULL,
    channel_name  TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    version       BIGINT NOT NULL DEFAULT 1,
    UNIQUE (connection_id, channel_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- The sources and sinks a team owns. One may belong to several teams, such
-- as a shared release channel.
CREATE TABLE team_sources (
    team_id   BIGINT NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    source_id BIGINT NOT NULL REFERENCES sources (id) ON DELETE CASCADE,
    PRIMARY KEY (team_id, source_id)
);

CREATE TABLE team_sinks (
    team_id BIGINT NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    sink_id BIGINT NOT NULL REFERENCES sinks (id) ON DELETE CASCADE,
    PRIMARY KEY (team_id, sink_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- A route forwards a team's source into one of its sinks (SYSREQ-046). The
-- composite keys make the database refuse a route between another team's
-- source or sink, and refuse to take a source or sink from a team while a
-- route uses it. Muted replies name the reply kinds the route does not post;
-- the board always updates (SYSREQ-047).
CREATE TABLE routes (
    id            BIGSERIAL PRIMARY KEY,
    team_id       BIGINT NOT NULL REFERENCES teams (id),
    source_id     BIGINT NOT NULL,
    sink_id       BIGINT NOT NULL,
    muted_replies JSONB NOT NULL DEFAULT '[]',
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    version       BIGINT NOT NULL DEFAULT 1,
    UNIQUE (team_id, source_id, sink_id),
    FOREIGN KEY (team_id, source_id) REFERENCES team_sources (team_id, source_id),
    FOREIGN KEY (team_id, sink_id) REFERENCES team_sinks (team_id, sink_id)
);
CREATE INDEX routes_source_idx ON routes (source_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE routes;
DROP TABLE team_sinks;
DROP TABLE team_sources;
DROP TABLE sinks;
DROP TABLE sources;
DROP TABLE team_operators;
DROP TABLE teams;
-- +goose StatementEnd
