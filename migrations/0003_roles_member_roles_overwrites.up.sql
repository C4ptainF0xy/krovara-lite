CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id    UUID REFERENCES spaces(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    permissions BIGINT DEFAULT 0,
    color       TEXT,
    position    INT DEFAULT 0,
    is_everyone BOOLEAN DEFAULT FALSE
);

CREATE TABLE member_roles (
    member_id UUID REFERENCES members(id) ON DELETE CASCADE,
    role_id   UUID REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (member_id, role_id)
);

CREATE TABLE channel_overwrites (
    channel_id UUID REFERENCES channels(id) ON DELETE CASCADE,
    role_id    UUID REFERENCES roles(id) ON DELETE CASCADE,
    allow      BIGINT DEFAULT 0,
    deny       BIGINT DEFAULT 0,
    PRIMARY KEY (channel_id, role_id)
);
