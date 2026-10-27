DROP TRIGGER IF EXISTS trg_audit ON crm.appointments;
DROP TRIGGER IF EXISTS trg_audit ON crm.clients;
DROP TRIGGER IF EXISTS trg_audit ON crm.payments;
DROP TRIGGER IF EXISTS trg_audit ON crm.barber_services;
DROP TRIGGER IF EXISTS trg_audit ON crm.loyalty_accounts;
DROP TRIGGER IF EXISTS trg_audit ON crm.work_shifts;
DROP SCHEMA IF EXISTS audit CASCADE;
