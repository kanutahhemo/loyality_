CREATE TABLE IF NOT EXISTS sp_users (
    uid serial PRIMARY KEY,
    login varchar(120) not null unique,
    password varchar(120),
    active bool default true
);

CREATE TABLE IF NOT EXISTS sp_statuses (
    status_id serial primary key,
    status_value varchar(32) UNIQUE
);

INSERT INTO sp_statuses (status_value) values ('REGISTERED'), ('INVALID'), ('PROCESSING'), ('PROCESSED') ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS sp_orders (
    order_id serial PRIMARY KEY,
    uid bigint,
    order_value int,
    created_time timestamp DEFAULT NOW(),
    status_id int,
    accrual float,
    CONSTRAINT fk_uid foreign key(uid) references sp_users(uid) on delete cascade,
    CONSTRAINT fk_status_id foreign key(status_id) references sp_statuses on delete cascade
);

CREATE TABLE IF NOT EXISTS sp_withdrawn_history (
    withdrawn_id serial PRIMARY KEY,
    uid bigint,
    withdrawn_value float,
    created_time timestamp DEFAULT NOW(),
    order_id bigint,
    CONSTRAINT fk_uid foreign key(uid) references sp_users(uid) on delete cascade
);

CREATE TABLE IF NOT EXISTS sp_requests_history (
    request_id serial PRIMARY KEY,
    uid bigint,
    created_time timestamp DEFAULT NOW(),
    CONSTRAINT fk_uid foreign key(uid) references sp_users(uid) on delete cascade
);
