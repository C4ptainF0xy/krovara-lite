ALTER TABLE users
    ADD COLUMN stripe_customer_id     TEXT,
    ADD COLUMN stripe_subscription_id TEXT,
    ADD COLUMN subscription_status    TEXT,
    ADD COLUMN current_period_end     TIMESTAMPTZ;

CREATE UNIQUE INDEX idx_users_stripe_customer_id ON users(stripe_customer_id)
    WHERE stripe_customer_id IS NOT NULL;
