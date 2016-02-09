-- +goose Up
create table server(url text not null primary key, username text not null, password text not null);
create table server_resource(uuid text not null primary key, name text not null, inuse integer not null, server text, FOREIGN KEY(server) REFERENCES server(url));

create table model(name text not null primary key);
create table model_file(name text not null primary key, model text not null, FOREIGN KEY(model) REFERENCES model(name));

create table template(name text not null primary key, file text not null);

-- +goose Down
drop table template;
drop table model_files;
drop table model;
drop table server_resource;
drop table server;
