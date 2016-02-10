-- +goose Up
ALTER TABLE server_resource ADD COLUMN device INTEGER;

-- +goose Down
ALTER TABLE server_resource RENAME TO server_resource_old;
CREATE TABLE server_resource(uuid text not null primary key, name text not null, inuse boolean not null, server_id integer not null, FOREIGN KEY(server_id) REFERENCES server(id));
INSERT INTO server_resource SELECT uuid, name, inuse, server_id FROM server_resource_old;
DROP TABLE server_resource_old;
