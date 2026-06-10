CREATE TABLE IF NOT EXISTS public.account (
    account_login varchar(255) NOT NULL,
    account_pass_sha256 varchar(64) NOT NULL,
    account_uuid uuid NOT NULL,
    account_create_dt timestamp DEFAULT now() NULL,
    account_balance numeric(15, 2) DEFAULT 0 NOT NULL,
    CONSTRAINT account_pk PRIMARY KEY (account_uuid),
    CONSTRAINT account_unique UNIQUE (account_login)
);

CREATE TABLE IF NOT EXISTS public.orders (
    orders_uuid uuid NOT NULL,
    orders_account_uuid uuid NOT NULL,
    orders_code int8 NOT NULL,
    orders_create_ts timestamp DEFAULT now() NOT NULL,
    orders_accrual numeric(15, 2) NULL,
    orders_status varchar(20) DEFAULT 'NEW' NULL,
    CONSTRAINT order_pk PRIMARY KEY (orders_uuid),
    CONSTRAINT orders_unique UNIQUE (orders_code)
);

CREATE INDEX IF NOT EXISTS orders_orders_account_uuid_idx ON public.orders USING btree (orders_account_uuid);
CREATE INDEX IF NOT EXISTS orders_orders_code_idx ON public.orders USING btree (orders_code);

CREATE TABLE IF NOT EXISTS public.withdrawals (
    withdrawals_uuid uuid NOT NULL,
    withdrawals_account_uuid uuid NOT NULL,
    withdrawals_sum numeric(15, 2) DEFAULT 0 NOT NULL,
    withdrawals_create_ts timestamp DEFAULT now() NOT NULL,
    withdrawals_order_number int8 NOT NULL,
    CONSTRAINT withdrawals_pk PRIMARY KEY (withdrawals_uuid)
);