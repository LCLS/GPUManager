-- +goose Up
create table job(id integer primary key, name text not null, model_id integer not null, template_id integer not null, count int not null, FOREIGN KEY(model_id) REFERENCES model(id), FOREIGN KEY(template_id) REFERENCES template(id));
create table job_instance(id integer primary key, completed boolean not null default 0, job_id integer not null, FOREIGN KEY(job_id) REFERENCES job(id));

-- +goose Down
drop table job_instance;
drop table job;
