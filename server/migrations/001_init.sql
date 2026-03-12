create table users (
  id varchar(64) primary key,
  email varchar(255) not null unique,
  password_hash varchar(255) not null,
  name varchar(120) not null,
  avatar_url text,
  status varchar(32) not null default 'active',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  last_login_at timestamptz
);

create table workspaces (
  id varchar(64) primary key,
  name varchar(120) not null,
  slug varchar(120) not null unique,
  owner_user_id varchar(64) not null references users(id),
  status varchar(32) not null default 'active',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table workspace_members (
  id bigserial primary key,
  workspace_id varchar(64) not null references workspaces(id),
  user_id varchar(64) not null references users(id),
  role varchar(32) not null,
  created_at timestamptz not null default now(),
  unique(workspace_id, user_id)
);

create table sessions (
  id varchar(64) primary key,
  workspace_id varchar(64) not null references workspaces(id),
  user_id varchar(64) not null references users(id),
  title varchar(255) not null default '未命名分析',
  status varchar(32) not null default 'active',
  last_run_id varchar(64),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now()
);

create table files (
  id varchar(64) primary key,
  workspace_id varchar(64) not null references workspaces(id),
  uploaded_by varchar(64) not null references users(id),
  display_name varchar(255) not null,
  content_type varchar(255),
  size_bytes bigint not null,
  storage_provider varchar(64) not null,
  bucket varchar(255),
  storage_key text not null,
  checksum varchar(128),
  status varchar(32) not null default 'uploaded',
  visibility varchar(32) not null default 'private',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz
);

create table session_files (
  session_id varchar(64) not null references sessions(id),
  file_id varchar(64) not null references files(id),
  created_at timestamptz not null default now(),
  primary key (session_id, file_id)
);

create table analysis_runs (
  id varchar(64) primary key,
  session_id varchar(64) not null references sessions(id),
  workspace_id varchar(64) not null references workspaces(id),
  user_id varchar(64) not null references users(id),
  status varchar(32) not null,
  input_message text not null,
  summary text not null default '',
  error_message text,
  report_file_id varchar(64),
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table reports (
  id varchar(64) primary key,
  run_id varchar(64) not null references analysis_runs(id),
  workspace_id varchar(64) not null references workspaces(id),
  title varchar(255) not null,
  author varchar(255),
  html_storage_provider varchar(64) not null,
  html_bucket varchar(255),
  html_storage_key text not null,
  snapshot_json jsonb not null,
  created_at timestamptz not null default now()
);
