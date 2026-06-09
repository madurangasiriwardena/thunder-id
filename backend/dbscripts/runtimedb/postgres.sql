-- Table to store OAuth2 authorization codes.
CREATE TABLE "AUTHORIZATION_CODE" (
    DEPLOYMENT_ID VARCHAR(255) NOT NULL,
    CODE_ID VARCHAR(36) PRIMARY KEY,
    AUTHORIZATION_CODE VARCHAR(500) NOT NULL,
    CLIENT_ID VARCHAR(255) NOT NULL,
    STATE VARCHAR(50) NOT NULL,
    AUTHZ_DATA JSONB NOT NULL,
    TIME_CREATED TIMESTAMP NOT NULL,
    EXPIRY_TIME TIMESTAMP NOT NULL
);

-- Composite index for authorization code lookup by code + deployment (hot login-path query)
CREATE INDEX idx_authorization_code_code_deployment ON "AUTHORIZATION_CODE" (AUTHORIZATION_CODE, DEPLOYMENT_ID);

-- Index for expiry time on AUTHORIZATION_CODE (supports cleanup and expiry checks)
CREATE INDEX idx_authz_code_expiry_time ON "AUTHORIZATION_CODE" (EXPIRY_TIME);

-- Table to store OAuth2 authorization request context
CREATE TABLE "AUTHORIZATION_REQUEST" (
    AUTH_ID VARCHAR(36) NOT NULL,
    DEPLOYMENT_ID VARCHAR(255) NOT NULL,
    REQUEST_DATA JSONB NOT NULL,
    EXPIRY_TIME TIMESTAMP NOT NULL,
    CREATED_AT TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (AUTH_ID, DEPLOYMENT_ID)
);

-- Index for expiry time on AUTHORIZATION_REQUEST (supports cleanup and expiry checks)
CREATE INDEX idx_authorization_request_expiry_time ON "AUTHORIZATION_REQUEST" (EXPIRY_TIME);

-- Table to store OAuth2 CIBA (Client-Initiated Backchannel Authentication) requests.
-- USER_ID is NULL at creation and populated at callback once the user authenticates.
-- EXECUTION_ID is intentionally omitted: it is transient, lives only in the notification
-- link URL and the FLOW_CONTEXT table, and is never needed for polling or token issuance.
CREATE TABLE "CIBA_AUTH_REQUEST" (
    AUTH_REQ_ID        VARCHAR(36)  NOT NULL,
    DEPLOYMENT_ID      VARCHAR(255) NOT NULL,
    CLIENT_ID          VARCHAR(255) NOT NULL,
    USER_ID            VARCHAR(36),
    STANDARD_SCOPES    TEXT         NOT NULL,
    STATE              VARCHAR(50)  NOT NULL,
    AUTHORIZED_SCOPES  TEXT,
    ATTRIBUTE_CACHE_ID VARCHAR(36),
    COMPLETED_ACR      VARCHAR(255),
    AUTH_TIME          TIMESTAMP,
    LAST_POLLED_AT     TIMESTAMP,
    EXPIRY_TIME        TIMESTAMP    NOT NULL,
    CREATED_AT         TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (AUTH_REQ_ID, DEPLOYMENT_ID)
);

-- Index for expiry time on CIBA_AUTH_REQUEST (supports cleanup and expiry checks)
CREATE INDEX idx_ciba_auth_request_expiry_time ON "CIBA_AUTH_REQUEST" (EXPIRY_TIME);

-- Table to store flow context
CREATE TABLE "FLOW_CONTEXT" (
    FLOW_ID VARCHAR(36) NOT NULL,
    DEPLOYMENT_ID VARCHAR(255) NOT NULL,
    CONTEXT JSONB,
    EXPIRY_TIME TIMESTAMP NOT NULL,
    CREATED_AT TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UPDATED_AT TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (FLOW_ID, DEPLOYMENT_ID)
);

-- Index for deployment isolation on FLOW_CONTEXT
CREATE INDEX idx_flow_context_deployment_id ON "FLOW_CONTEXT" (DEPLOYMENT_ID);

-- Index for expiry time on FLOW_CONTEXT
CREATE INDEX idx_flow_context_expiry_time ON "FLOW_CONTEXT" (EXPIRY_TIME);

-- Table to store WebAuthn session data
CREATE TABLE "WEBAUTHN_SESSION" (
    SESSION_KEY VARCHAR(255) NOT NULL,
    DEPLOYMENT_ID VARCHAR(255) NOT NULL,
    SESSION_DATA JSONB NOT NULL,
    CREATED_AT TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    EXPIRY_TIME TIMESTAMP NOT NULL,
    PRIMARY KEY (SESSION_KEY, DEPLOYMENT_ID)
);

-- Index for expiry time on WEBAUTHN_SESSION
CREATE INDEX idx_webauthn_session_expiry_time ON "WEBAUTHN_SESSION" (EXPIRY_TIME);

-- Table to store attribute cache entries
CREATE TABLE "ATTRIBUTE_CACHE" (
    ID VARCHAR(36) PRIMARY KEY,
    DEPLOYMENT_ID VARCHAR(255) NOT NULL,
    ATTRIBUTES JSONB NOT NULL,
    EXPIRY_TIME TIMESTAMP NOT NULL,
    CREATED_AT TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Table to store pushed authorization requests (PAR)
CREATE TABLE "PAR_REQUEST" (
    REQUEST_URI VARCHAR(43) PRIMARY KEY,
    DEPLOYMENT_ID VARCHAR(255) NOT NULL,
    REQUEST_PARAMS JSONB NOT NULL,
    EXPIRY_TIME TIMESTAMP NOT NULL
);

-- Index for expiry time on PAR_REQUEST (supports cleanup and expiry checks)
CREATE INDEX idx_par_request_expiry_time ON "PAR_REQUEST" (EXPIRY_TIME);

-- Table to store JWT jti values for replay protection across consumers. Rows are isolated by NAMESPACE.
CREATE TABLE "JTI_RECORD" (
    DEPLOYMENT_ID VARCHAR(255) NOT NULL,
    NAMESPACE VARCHAR(64) NOT NULL,
    JTI VARCHAR(256) NOT NULL,
    EXPIRY_TIME TIMESTAMP NOT NULL,
    PRIMARY KEY (DEPLOYMENT_ID, NAMESPACE, JTI)
);

-- Index for expiry time on JTI_RECORD (supports cleanup and expiry checks)
CREATE INDEX idx_jti_record_expiry_time ON "JTI_RECORD" (EXPIRY_TIME);

-- Table to store browser SSO session records. SESSION_ID is internal only; HANDLE_ID
-- is the opaque handle sent to the client via the __Host-tid_session cookie.
CREATE TABLE "SESSION_RECORD" (
    DEPLOYMENT_ID       VARCHAR(255) NOT NULL,
    SESSION_ID          VARCHAR(36)  NOT NULL,
    SUBJECT_ID          VARCHAR(255) NOT NULL,
    SESSION_GROUP_ID    VARCHAR(255) NOT NULL,
    AUTHENTICATED_AT    TIMESTAMP    NOT NULL,
    ASSURANCE_LEVEL     VARCHAR(255) NOT NULL,
    CREATED_AT          TIMESTAMP    NOT NULL,
    LAST_ACTIVE_AT      TIMESTAMP    NOT NULL,
    IDLE_EXPIRES_AT     TIMESTAMP    NOT NULL,
    ABSOLUTE_EXPIRES_AT TIMESTAMP    NOT NULL,
    HANDLE_ID           VARCHAR(36)  NOT NULL,
    HANDLE_ISSUED_AT    TIMESTAMP    NOT NULL,
    HANDLE_EXPIRES_AT   TIMESTAMP    NOT NULL,
    BINDING_TYPE        VARCHAR(64)  NOT NULL,
    SESSION_STATE       VARCHAR(32)  NOT NULL,
    VERSION             INTEGER      NOT NULL DEFAULT 0,
    PRIMARY KEY (SESSION_ID, DEPLOYMENT_ID)
);

-- Unique index on HANDLE_ID scoped to DEPLOYMENT_ID (the cookie lookup hot path)
CREATE UNIQUE INDEX idx_session_record_handle ON "SESSION_RECORD" (HANDLE_ID, DEPLOYMENT_ID);

-- Index for subject-based session lookup
CREATE INDEX idx_session_record_subject ON "SESSION_RECORD" (SUBJECT_ID, DEPLOYMENT_ID);

-- Index for expiry-based cleanup jobs
CREATE INDEX idx_session_record_absolute_expiry ON "SESSION_RECORD" (ABSOLUTE_EXPIRES_AT);

-- Index for find-or-create lookup (GetActiveSessionBySubjectAndGroup hot path)
CREATE INDEX idx_session_record_subject_group ON "SESSION_RECORD" (SUBJECT_ID, SESSION_GROUP_ID, SESSION_STATE, DEPLOYMENT_ID);

CREATE TABLE "CLIENT_SESSION" (
    DEPLOYMENT_ID      VARCHAR(255) NOT NULL,
    CLIENT_SESSION_ID  VARCHAR(36)  NOT NULL,
    SESSION_ID         VARCHAR(36)  NOT NULL,
    CLIENT_ID          VARCHAR(255) NOT NULL,
    OIDC_SID           VARCHAR(36)  NOT NULL,
    CREATED_AT         TIMESTAMP    NOT NULL,
    LAST_USED_AT       TIMESTAMP    NOT NULL,
    STATUS             VARCHAR(32)  NOT NULL,
    GRANTED_SCOPES     TEXT         NOT NULL DEFAULT '',
    VERSION            INTEGER      NOT NULL DEFAULT 0,
    PRIMARY KEY (CLIENT_SESSION_ID, DEPLOYMENT_ID),
    FOREIGN KEY (SESSION_ID, DEPLOYMENT_ID) REFERENCES "SESSION_RECORD" (SESSION_ID, DEPLOYMENT_ID) ON DELETE CASCADE
);

-- Unique index for per-app session lookup (the EnsureClientSession hot path)
CREATE UNIQUE INDEX idx_client_session_session_client ON "CLIENT_SESSION" (SESSION_ID, CLIENT_ID, DEPLOYMENT_ID);

-- Index for OIDC front-channel logout (sid lookup)
CREATE INDEX idx_client_session_oidc_sid ON "CLIENT_SESSION" (OIDC_SID, DEPLOYMENT_ID);
