-- +goose Up
ALTER TABLE job_instance ADD COLUMN resource_id text DEFAULT "";

-- +goose Down
ALTER TABLE job_instance RENAME TO job_instance_old;
CREATE TABLE job_instance(id integer primary key, completed boolean not null default 0, job_id integer not null, pid integer default -1, FOREIGN KEY(job_id) REFERENCES job(id));
INSERT INTO job_instance SELECT id, completed, job_id, pid FROM job_instance_old;
DROP TABLE job_instance_old;
