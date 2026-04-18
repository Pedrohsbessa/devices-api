CREATE TABLE devices (
    id         UUID        PRIMARY KEY,
    name       TEXT        NOT NULL,
    brand      TEXT        NOT NULL,
    state      TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT devices_state_valid   CHECK (state IN ('available', 'in-use', 'inactive')),
    CONSTRAINT devices_name_present  CHECK (btrim(name)  <> ''),
    CONSTRAINT devices_brand_present CHECK (btrim(brand) <> '')
);

-- Two single-column indexes mirror the "fetch by brand" and "fetch by state"
-- endpoints required by the specification. Queries combining both filters
-- are resolved by PostgreSQL via bitmap AND; a composite index is deferred
-- to a future iteration if profiling identifies that query as a hot path.
CREATE INDEX idx_devices_brand ON devices (brand);
CREATE INDEX idx_devices_state ON devices (state);
