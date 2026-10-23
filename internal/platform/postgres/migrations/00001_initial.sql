-- +goose Up
CREATE TABLE users (
    user_id uuid PRIMARY KEY,
    email varchar(320) NOT NULL,
    password_hash text NOT NULL,
    role varchar(16) NOT NULL CHECK (role IN ('admin', 'user')),
    email_verified_at timestamptz NOT NULL,
    invited_by_user_id uuid REFERENCES users(user_id),
    default_workspace_id uuid,
    max_single_file_bytes bigint NOT NULL DEFAULT 50000000 CHECK (max_single_file_bytes >= 0),
    storage_quota_bytes bigint NOT NULL DEFAULT 500000000 CHECK (storage_quota_bytes >= 0),
    storage_used_bytes bigint NOT NULL DEFAULT 0 CHECK (storage_used_bytes >= 0),
    url_link_quota integer NOT NULL DEFAULT 100 CHECK (url_link_quota >= 0),
    minio_asset_link_quota integer NOT NULL DEFAULT 100 CHECK (minio_asset_link_quota >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_lower_uq ON users (lower(email));

CREATE TABLE workspaces (
    workspace_id uuid PRIMARY KEY,
    owner_user_id uuid NOT NULL UNIQUE REFERENCES users(user_id),
    name varchar(120) NOT NULL DEFAULT '默认空间',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE users ADD CONSTRAINT users_default_workspace_fk
    FOREIGN KEY (default_workspace_id) REFERENCES workspaces(workspace_id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE system_settings (
    settings_id smallint PRIMARY KEY CHECK (settings_id = 1),
    allow_public_registration boolean NOT NULL DEFAULT false,
    allow_invite_registration boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid REFERENCES users(user_id)
);
INSERT INTO system_settings (settings_id) VALUES (1);

CREATE TABLE email_verification_codes (
    verification_id uuid PRIMARY KEY,
    email varchar(320) NOT NULL,
    purpose varchar(24) NOT NULL CHECK (purpose IN ('register', 'password_reset')),
    code_hash char(64),
    delivery_status varchar(16) NOT NULL CHECK (delivery_status IN ('pending', 'sent', 'suppressed', 'failed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    attempt_count smallint NOT NULL DEFAULT 0 CHECK (attempt_count BETWEEN 0 AND 5),
    source_ip inet,
    CHECK ((delivery_status IN ('pending', 'sent') AND code_hash IS NOT NULL) OR (delivery_status IN ('suppressed', 'failed') AND code_hash IS NULL))
);
CREATE INDEX email_codes_email_purpose_created_idx ON email_verification_codes (email, purpose, created_at DESC);
CREATE INDEX email_codes_source_created_idx ON email_verification_codes (source_ip, created_at DESC);

CREATE TABLE user_sessions (
    session_id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(user_id),
    token_hash char(64) NOT NULL UNIQUE,
    client_app_id varchar(64) NOT NULL,
    device_instance_id varchar(128),
    device_name varchar(120),
    platform varchar(32),
    source_ip inet,
    user_agent text,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CHECK (expires_at <= created_at + interval '7 days')
);
CREATE INDEX user_sessions_user_created_idx ON user_sessions (user_id, created_at DESC);
CREATE INDEX user_sessions_active_idx ON user_sessions (expires_at) WHERE revoked_at IS NULL;

CREATE TABLE user_permissions (
    user_id uuid NOT NULL REFERENCES users(user_id),
    permission_code varchar(64) NOT NULL,
    granted_at timestamptz,
    granted_by uuid REFERENCES users(user_id),
    revoked_at timestamptz,
    revoked_by uuid REFERENCES users(user_id),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, permission_code),
    CHECK ((revoked_at IS NULL AND granted_at IS NOT NULL) OR (revoked_at IS NOT NULL AND granted_at IS NULL))
);

CREATE TABLE invite_codes (
    invite_code_id uuid PRIMARY KEY,
    code_hash char(64) NOT NULL UNIQUE,
    created_by_user_id uuid NOT NULL REFERENCES users(user_id),
    assigned_role varchar(16) NOT NULL CHECK (assigned_role IN ('admin', 'user')),
    bound_email varchar(320),
    expires_at timestamptz NOT NULL,
    redeemed_at timestamptz,
    redeemed_by_user_id uuid REFERENCES users(user_id),
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at AND expires_at <= created_at + interval '7 days'),
    CHECK ((redeemed_at IS NULL AND redeemed_by_user_id IS NULL) OR (redeemed_at IS NOT NULL AND redeemed_by_user_id IS NOT NULL)),
    CHECK (assigned_role <> 'admin' OR bound_email IS NOT NULL)
);
CREATE INDEX invite_codes_creator_status_idx ON invite_codes (created_by_user_id, expires_at) WHERE redeemed_at IS NULL AND revoked_at IS NULL;

CREATE TABLE business_integrations (
    business_id varchar(64) PRIMARY KEY,
    display_name varchar(120) NOT NULL,
    status varchar(16) NOT NULL CHECK (status IN ('active', 'disabled')),
    bucket_name varchar(63) NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_business_access (
    user_id uuid NOT NULL REFERENCES users(user_id),
    business_id varchar(64) NOT NULL REFERENCES business_integrations(business_id),
    granted_by uuid NOT NULL REFERENCES users(user_id),
    granted_at timestamptz NOT NULL,
    revoked_at timestamptz,
    PRIMARY KEY (user_id, business_id)
);

CREATE TABLE rule_templates (
    template_id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces(workspace_id),
    name varchar(120) NOT NULL,
    description text NOT NULL DEFAULT '',
    status varchar(16) NOT NULL CHECK (status IN ('active', 'disabled')),
    latest_version integer NOT NULL CHECK (latest_version > 0),
    deleted_at timestamptz,
    created_by uuid NOT NULL REFERENCES users(user_id),
    updated_by uuid NOT NULL REFERENCES users(user_id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX rule_templates_workspace_name_uq ON rule_templates (workspace_id, lower(name));
CREATE INDEX rule_templates_workspace_updated_idx ON rule_templates (workspace_id, status, updated_at DESC);

CREATE TABLE rule_template_versions (
    template_id uuid NOT NULL REFERENCES rule_templates(template_id),
    version integer NOT NULL CHECK (version > 0),
    rules jsonb NOT NULL,
    change_note text NOT NULL DEFAULT '',
    created_by uuid NOT NULL REFERENCES users(user_id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (template_id, version)
);

CREATE TABLE assets (
    asset_id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces(workspace_id),
    owner_user_id uuid NOT NULL REFERENCES users(user_id),
    business_id varchar(64) NOT NULL REFERENCES business_integrations(business_id),
    logical_path text NOT NULL,
    bucket varchar(63) NOT NULL,
    object_key text NOT NULL,
    original_name text NOT NULL,
    content_type varchar(255) NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    checksum_sha256 char(64),
    deleted_at timestamptz,
    purged_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (bucket, object_key)
);
CREATE INDEX assets_workspace_created_idx ON assets (workspace_id, created_at DESC);

CREATE TABLE asset_deletion_records (
    deletion_id uuid PRIMARY KEY,
    asset_id uuid NOT NULL REFERENCES assets(asset_id),
    workspace_id uuid NOT NULL REFERENCES workspaces(workspace_id),
    deleted_by uuid NOT NULL REFERENCES users(user_id),
    deleted_at timestamptz NOT NULL,
    purge_after timestamptz NOT NULL,
    status varchar(16) NOT NULL CHECK (status IN ('pending', 'restored', 'purged')),
    purged_at timestamptz,
    last_error text,
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX asset_deletion_due_idx ON asset_deletion_records (status, purge_after);
CREATE UNIQUE INDEX asset_deletion_pending_uq ON asset_deletion_records (asset_id) WHERE status = 'pending';

CREATE TABLE short_links (
    link_id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces(workspace_id),
    target_type varchar(24) NOT NULL CHECK (target_type IN ('url', 'minio_asset')),
    target_url text,
    asset_id uuid REFERENCES assets(asset_id),
    expires_at timestamptz,
    max_visits bigint NOT NULL DEFAULT -1 CHECK (max_visits >= -1),
    visits_used bigint NOT NULL DEFAULT 0 CHECK (visits_used >= 0),
    allowed_ip_cidrs cidr[] NOT NULL DEFAULT '{}',
    require_password boolean NOT NULL DEFAULT false,
    password_hash text,
    wrong_password_policy jsonb,
    status varchar(16) NOT NULL CHECK (status IN ('enabled', 'disabled')),
    template_id uuid,
    template_version integer,
    created_by uuid NOT NULL REFERENCES users(user_id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (template_id, template_version) REFERENCES rule_template_versions(template_id, version),
    CHECK ((target_type = 'url' AND target_url IS NOT NULL AND asset_id IS NULL) OR (target_type = 'minio_asset' AND target_url IS NULL AND asset_id IS NOT NULL)),
    CHECK ((require_password AND password_hash IS NOT NULL AND wrong_password_policy IS NOT NULL) OR (NOT require_password AND password_hash IS NULL AND wrong_password_policy IS NULL)),
    CHECK ((template_id IS NULL AND template_version IS NULL) OR (template_id IS NOT NULL AND template_version IS NOT NULL))
);
CREATE INDEX short_links_workspace_created_idx ON short_links (workspace_id, created_at DESC);
CREATE INDEX short_links_owner_target_idx ON short_links (created_by, target_type);

CREATE TABLE short_link_codes (
    code varchar(64) PRIMARY KEY,
    link_id uuid NOT NULL REFERENCES short_links(link_id),
    status varchar(16) NOT NULL CHECK (status IN ('active', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);
CREATE UNIQUE INDEX short_link_codes_one_active_uq ON short_link_codes (link_id) WHERE status = 'active';

CREATE TABLE short_link_access_logs (
    event_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    link_id uuid NOT NULL REFERENCES short_links(link_id),
    workspace_id uuid NOT NULL REFERENCES workspaces(workspace_id),
    business_id varchar(64),
    event_type varchar(24) NOT NULL CHECK (event_type IN ('link_access', 'password_attempt')),
    accessed_at timestamptz NOT NULL DEFAULT now(),
    source_ip inet,
    user_agent text,
    result varchar(16) NOT NULL CHECK (result IN ('allowed', 'denied', 'challenge')),
    reason varchar(40),
    consumes_visit boolean NOT NULL,
    request_id text
);
CREATE INDEX short_link_logs_link_accessed_idx ON short_link_access_logs (link_id, accessed_at DESC);
CREATE INDEX short_link_logs_workspace_accessed_idx ON short_link_access_logs (workspace_id, accessed_at DESC);

CREATE TABLE short_link_password_attempt_state (
    link_id uuid NOT NULL REFERENCES short_links(link_id),
    source_ip inet NOT NULL,
    window_started_at timestamptz NOT NULL,
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
    last_failed_at timestamptz NOT NULL,
    cooldown_level smallint NOT NULL DEFAULT 0 CHECK (cooldown_level BETWEEN 0 AND 3),
    locked_until timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (link_id, source_ip)
);

CREATE TABLE unknown_short_code_events (
    event_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code_fingerprint char(64) NOT NULL,
    window_started_at timestamptz NOT NULL,
    first_occurred_at timestamptz NOT NULL,
    last_occurred_at timestamptz NOT NULL,
    event_count bigint NOT NULL DEFAULT 1 CHECK (event_count > 0),
    source_ip inet NOT NULL,
    user_agent text,
    request_id text,
    rate_limited boolean NOT NULL DEFAULT false,
    UNIQUE (source_ip, code_fingerprint, window_started_at)
);
CREATE INDEX unknown_codes_last_occurred_idx ON unknown_short_code_events (last_occurred_at DESC);

CREATE TABLE audit_logs (
    event_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id uuid REFERENCES users(user_id),
    workspace_id uuid REFERENCES workspaces(workspace_id),
    event_type varchar(80) NOT NULL,
    target_type varchar(40),
    target_id text,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    source_ip inet,
    user_agent text,
    result varchar(16) NOT NULL CHECK (result IN ('success', 'failure')),
    details jsonb NOT NULL DEFAULT '{}',
    request_id text
);
CREATE INDEX audit_logs_actor_occurred_idx ON audit_logs (actor_user_id, occurred_at DESC);
CREATE INDEX audit_logs_workspace_occurred_idx ON audit_logs (workspace_id, occurred_at DESC);

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS unknown_short_code_events;
DROP TABLE IF EXISTS short_link_password_attempt_state;
DROP TABLE IF EXISTS short_link_access_logs;
DROP TABLE IF EXISTS short_link_codes;
DROP TABLE IF EXISTS short_links;
DROP TABLE IF EXISTS asset_deletion_records;
DROP TABLE IF EXISTS assets;
DROP TABLE IF EXISTS rule_template_versions;
DROP TABLE IF EXISTS rule_templates;
DROP TABLE IF EXISTS user_business_access;
DROP TABLE IF EXISTS business_integrations;
DROP TABLE IF EXISTS invite_codes;
DROP TABLE IF EXISTS user_permissions;
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS email_verification_codes;
DROP TABLE IF EXISTS system_settings;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_default_workspace_fk;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS users;
