# Migrations

Manual SQL migrations — **AutoMigrate is disabled in this project.**

- One file per change, named `YYYYMMDD_<description>.sql`
- Tables: InnoDB, utf8mb4, use `CREATE TABLE IF NOT EXISTS`
- Run manually on the server:

```bash
mysql -u pdlife_pdlife-db-admin -p pdlife_pdlife-db < migrations/YYYYMMDD_name.sql
```

Schema definitions live in [docs/schema_spec.md](../docs/schema_spec.md).
