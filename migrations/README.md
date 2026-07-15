# Migrations

Manual SQL migrations — **AutoMigrate is disabled in this project.**

- One file per change, named `YYYYMMDD_<description>.sql`
- Tables: InnoDB, utf8mb4, use `CREATE TABLE IF NOT EXISTS`
- Run manually on the server:

```bash
mysql -u pdlife_pdlife-db-admin -p pdlife_pdlife-db < migrations/YYYYMMDD_name.sql
```

Schema definitions live in [docs/schema_spec.md](../docs/schema_spec.md).

## Applied so far (run in this order)

1. `20260706_create_users_email_verifications_patient_profiles.sql` — creates `users`, `email_verifications`, `patient_profiles`
2. `20260706_rename_password_to_password_hash.sql` — renames `users.password` to `users.password_hash`
3. `20260707_add_refresh_and_password_reset.sql` — adds `users.security_stamp`, `refresh_tokens`, `password_reset_tokens`
4. `20260708_create_apd_log_book.sql` — creates `apd_prescriptions`, `apd_log_entries`
5. `20260709_create_foodcheck.sql` — creates the `foodcheck_*` tables/views (Food Check port, see [docs/foodcheck_survey.md](../docs/foodcheck_survey.md)); data populated afterward by `cmd/migrate_foodcheck`
6. `20260709_fix_foodcheck_deriv_by_length.sql` — widens `foodcheck_food_nutrients.deriv_by` to VARCHAR(150) (the real source data has values up to 95 chars; migration #5 guessed VARCHAR(30) from short examples and the real import hit "data too long")
7. `20260710_create_foodcheck_source_snapshots.sql` — creates `foodcheck_source_snapshots` (Food Check Phase 3, monthly drift-check bookkeeping; see `cmd/foodcheck_diffcheck`) — never written to by the web app

Once a migration has run on production, never edit it — write a new file for any further change.
