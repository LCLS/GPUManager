-- +goose Up
create table server(id integer primary key, url text not null, username text not null, password text not null, enabled boolean not null default 1);
create table server_resource(uuid text not null primary key, name text not null, inuse boolean not null, server_id integer not null, FOREIGN KEY(server_id) REFERENCES server(id));

create table model(id integer primary key, name text not null);
create table model_file(id integer primary key, name text not null, model_id integer not null, FOREIGN KEY(model_id) REFERENCES model(id));

create table template(id integer primary key, name text not null, file text not null);

-- +goose Down
drop table template;
drop table model_files;
drop table model;
drop table server_resource;
drop table server;
