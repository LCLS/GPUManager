-- +goose Up
ALTER TABLE server ADD COLUMN wdir TEXT DEFAULT "";

-- +goose Down
ALTER TABLE server RENAME TO server_old;
CREATE TABLE server (id integer primary key, url text not null, username text not null, password text not null, enabled boolean not null default 1);
INSERT INTO server SELECT id, url, username, password, enabled FROM server_old;
DROP TABLE server_old;
