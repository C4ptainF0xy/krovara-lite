-- name: SetUserStripeCustomer :exec
UPDATE users SET stripe_customer_id = $2 WHERE id = $1;

-- name: GetUserByStripeCustomer :one
SELECT * FROM users WHERE stripe_customer_id = $1;

-- name: SetUserSubscription :exec
UPDATE users
   SET stripe_subscription_id = sqlc.narg('stripe_subscription_id'),
       subscription_status    = sqlc.narg('subscription_status'),
       current_period_end     = sqlc.narg('current_period_end'),
       tier                   = sqlc.arg('tier')
 WHERE stripe_customer_id = sqlc.arg('stripe_customer_id');
