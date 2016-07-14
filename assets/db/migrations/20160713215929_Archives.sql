-- +goose Up
CREATE TABLE archive (id integer primary key, url text not null, wdir text not null, username text not null, password text not null, used integer not null, total integer not null, enabled boolean not null default 1);

-- +goose Down
DROP TABLE archive;

