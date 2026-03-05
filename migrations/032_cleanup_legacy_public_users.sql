BEGIN;

-- Point legacy audit FKs to user.users before dropping public.users.
ALTER TABLE IF EXISTS audit.audit_logs
  DROP CONSTRAINT IF EXISTS audit_logs_admin_id_fkey;

ALTER TABLE IF EXISTS audit.audit_logs
  ADD CONSTRAINT audit_logs_admin_id_fkey
  FOREIGN KEY (admin_id) REFERENCES "user".users(id) ON DELETE RESTRICT;

ALTER TABLE IF EXISTS audit.reports
  DROP CONSTRAINT IF EXISTS reports_user_id_fkey;

ALTER TABLE IF EXISTS audit.reports
  ADD CONSTRAINT reports_user_id_fkey
  FOREIGN KEY (user_id) REFERENCES "user".users(id) ON DELETE CASCADE;

-- Drop legacy table after references are rewired.
DROP TABLE IF EXISTS public.users;

COMMIT;
