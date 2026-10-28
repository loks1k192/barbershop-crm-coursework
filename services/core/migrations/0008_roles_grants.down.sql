-- Роли не удаляются (могут владеть объектами в других БД кластера) — только права.
REVOKE ALL ON ALL TABLES IN SCHEMA crm, audit, staging FROM crm_app, crm_readonly;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA crm, staging FROM crm_app;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA crm, audit, staging FROM crm_app, crm_readonly;
REVOKE ALL ON SCHEMA crm, audit, staging FROM crm_app, crm_readonly;
ALTER DEFAULT PRIVILEGES IN SCHEMA crm REVOKE SELECT ON TABLES FROM crm_app;
