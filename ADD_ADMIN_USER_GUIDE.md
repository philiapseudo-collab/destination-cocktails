# Adding Admin User 254708116809

## Problem
You're getting the error: `unauthorized: admin user not found or inactive` when trying to log in with phone number `254708116809`.

## Solution
You need to add this phone number to the `admin_users` table in your PostgreSQL database.

---

## **Option 1: Using Railway CLI (Recommended for Production)**

### Step 1: Connect to Railway Database
```bash
# In your destination-cocktails directory
railway connect
```

### Step 2: Run the migration
```bash
go run cmd/run_migration/main.go migrations/006_add_admin_254708116809.sql
```

**OR** if you prefer the quick SQL file:
```bash
# Connect to Railway's PostgreSQL
railway run psql -f add_admin_quick.sql
```

---

## **Option 2: Direct SQL via Railway Web Console**

1. Go to your Railway project: https://railway.app
2. Click on your **PostgreSQL** service
3. Go to the **Query** tab
4. Run this SQL:

```sql
INSERT INTO admin_users (phone_number, name, role, is_active)
VALUES ('254708116809', 'Manager', 'OWNER', true)
ON CONFLICT (phone_number) DO UPDATE SET
    is_active = true,
    role = 'OWNER';

-- Verify it was added
SELECT * FROM admin_users WHERE phone_number = '254708116809';
```

---

## **Option 3: Using Railway CLI Direct Query**

```bash
railway run psql -c "INSERT INTO admin_users (phone_number, name, role, is_active) VALUES ('254708116809', 'Manager', 'OWNER', true) ON CONFLICT (phone_number) DO UPDATE SET is_active = true, role = 'OWNER';"
```

---

## **Option 4: Local Testing (if you have local DB)**

If you're running the database locally:

```bash
# Run the migration
go run cmd/run_migration/main.go migrations/006_add_admin_254708116809.sql

# OR use psql directly
psql -U postgres -d destination_cocktails -f add_admin_quick.sql
```

---

## Files Created

1. **`migrations/006_add_admin_254708116809.sql`** - Migration file (version controlled)
2. **`add_admin_quick.sql`** - Quick SQL snippet for direct execution
3. **`cmd/run_migration/main.go`** - General migration runner (reusable)

---

## Verification

After running any of the above options, verify the admin was added:

```sql
SELECT * FROM admin_users WHERE phone_number = '254708116809';
```

You should see:
- **phone_number**: 254708116809
- **name**: Manager
- **role**: OWNER
- **is_active**: true

---

## Next Steps

1. Run one of the options above to add the admin user
2. Try logging in again with phone `254708116809`
3. You should now be able to receive and verify the OTP

---

## Admin Users Table Schema

```sql
CREATE TABLE admin_users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    phone_number VARCHAR(20) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'MANAGER',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

**Roles**: OWNER, MANAGER
**Note**: OTP codes are stored in a separate `otp_codes` table and are generated dynamically during login.
