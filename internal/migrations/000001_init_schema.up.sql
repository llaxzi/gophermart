CREATE SCHEMA gophermart;

CREATE TYPE gophermart.status AS ENUM('NEW','PROCESSING','INVALID','PROCESSED');

CREATE TABLE gophermart.users(
    login VARCHAR (50) PRIMARY KEY,
    password VARCHAR(255),
    balance_current DOUBLE PRECISION,
    balance_withdrawn DOUBLE PRECISION
);

CREATE TABLE gophermart.orders(
    number BIGINT PRIMARY KEY,
    login VARCHAR(50),
    status gophermart.status,
    accrual double precision,
    uploaded_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT fk FOREIGN KEY (login) REFERENCES gophermart.users(login)
);

CREATE TABLE gophermart.withdrawals(
    order_id BIGINT PRIMARY KEY,
    login varchar(50),
    sum double precision,
    processed_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT fk FOREIGN KEY (login) REFERENCES gophermart.users(login)
);

