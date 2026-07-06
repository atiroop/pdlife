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

Once a migration has run on production, never edit it — write a new file for any further change.
