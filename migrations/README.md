# Migrations

Manual SQL migrations — **AutoMigrate is disabled in this project.**

- One file per change, named `YYYYMMDD_<description>.sql`
- Tables: InnoDB, utf8mb4, use `CREATE TABLE IF NOT EXISTS`
- Run manually on the server:

```bash
mysql -u pdlife_pdlife-db-admin -p pdlife_pdlife-db < migrations/YYYYMMDD_name.sql
```

Schema definitions live in [docs/schema_spec.md](../docs/schema_spec.md).

## Local development database

`DB_HOST=localhost` in `.env` points at the MariaDB running in Docker
Desktop (container `nhe-mariadb-dev`, shared with the nhe.one project).
One MariaDB server hosts many databases, so pdlife simply gets its own
alongside nhe's — there is no port to change and nothing of nhe's is
touched.

To create it and run every migration into it, from the repo root:

```powershell
./scripts/setup-dev-db.ps1
```

Safe to re-run. Food Check's tables are created empty; populate them with
`go run ./cmd/migrate_foodcheck`.

## Applied so far (run in this order)

Filename order is the intended order — where two share a date, the
alphabetical tie-break already puts them right.

1. `20260706_create_users_email_verifications_patient_profiles.sql` — creates `users`, `email_verifications`, `patient_profiles`
2. `20260706_rename_password_to_password_hash.sql` — renames `users.password` to `users.password_hash`
3. `20260707_add_refresh_and_password_reset.sql` — adds `users.security_stamp`, `refresh_tokens`, `password_reset_tokens`
4. `20260708_create_apd_log_book.sql` — creates `apd_prescriptions`, `apd_log_entries`
5. `20260709_create_foodcheck.sql` — creates the `foodcheck_*` tables/views (Food Check port, see [docs/foodcheck_survey.md](../docs/foodcheck_survey.md)); data populated afterward by `cmd/migrate_foodcheck`
6. `20260709_fix_foodcheck_deriv_by_length.sql` — widens `foodcheck_food_nutrients.deriv_by` to VARCHAR(150) (the real source data has values up to 95 chars; migration #5 guessed VARCHAR(30) from short examples and the real import hit "data too long")
7. `20260710_create_capd_log_book.sql` — creates `capd_log_entries`
8. `20260710_create_foodcheck_source_snapshots.sql` — creates `foodcheck_source_snapshots` (Food Check Phase 3, monthly drift-check bookkeeping; see `cmd/foodcheck_diffcheck`) — never written to by the web app
9. `20260710_create_news_articles.sql` — creates `news_articles`
10. `20260711_add_review_fields_to_news_articles.sql` — adds the admin review-queue columns
11. `20260712_add_feature_image_to_news_articles.sql` — adds `feature_image_url`
12. `20260713_add_health_data_consent_to_patient_profiles.sql` — adds the PDPA health-data consent columns
13. `20260714_create_editorial_articles.sql` — creates `editorial_articles`
14. `20260715_add_account_deletion_to_users.sql` — adds the deletion-request columns
15. `20260716_create_hd_log_book.sql` — creates `hd_log_entries`
16. `20260717_add_terms_accepted_to_users.sql` — adds `terms_accepted_at`, `terms_accepted_version`
17. `20260718_create_lab_results.sql` — creates `lab_results`
18. `20260719_create_admin_action_logs_and_user_suspension.sql` — creates `admin_action_logs`, adds the suspension columns to `users`
19. `20260720_add_cycle_number_to_apd_log_entries.sql` — adds `cycle_number` for per-cycle APD logging

Once a migration has run on production, never edit it — write a new file for any further change.
