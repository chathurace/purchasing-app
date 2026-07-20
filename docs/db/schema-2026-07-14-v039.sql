-- =============================================================================
--  Purchasing App — PostgreSQL schema snapshot
-- =============================================================================
--
--  ⚠️  FOR HUMAN INSPECTION ONLY — DO NOT USE TO CREATE/RESTORE A DATABASE.  ⚠️
--
--  This is a read-only reference dump of the `purchasing` database schema as it
--  stands AFTER applying all migrations through 039_recommendation_assignee.sql.
--  It is NOT the source of truth and is NOT applied anywhere: the real schema is
--  built exclusively by the numbered migrations in backend/migrations/. Do not
--  run this file against any database and do not edit it by hand — to change the
--  schema, add a new migration.
--
--  Generated : 2026-07-14
--  Version    : v039 (latest migration: 039_recommendation_assignee.sql)
--  Source     : pg_dump --schema-only  (local dev DB `purchasing`)
-- =============================================================================

--
-- PostgreSQL database dump
--

\restrict rmtS88jEiFfF9p0g3Xt3X2hRSQcqGbg6UkWmKqN9MdbZN5D75ItKcpTvDyZXYog

-- Dumped from database version 17.8 (Homebrew)
-- Dumped by pg_dump version 17.8 (Homebrew)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: audit_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audit_events (
    id bigint NOT NULL,
    action text NOT NULL,
    qualifier text DEFAULT ''::text NOT NULL,
    entity_type text NOT NULL,
    entity_id bigint,
    detail text DEFAULT ''::text NOT NULL,
    actor_id bigint,
    actor_email text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: audit_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.audit_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: audit_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.audit_events_id_seq OWNED BY public.audit_events.id;


--
-- Name: config_options; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.config_options (
    id bigint NOT NULL,
    list_key text NOT NULL,
    value text NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: config_options_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.config_options_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: config_options_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.config_options_id_seq OWNED BY public.config_options.id;


--
-- Name: contracts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contracts (
    id bigint NOT NULL,
    purchase_request_id bigint NOT NULL,
    quotation_id bigint,
    vendor_id bigint NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    total_amount numeric(16,2) DEFAULT 0 NOT NULL,
    currency text DEFAULT 'USD'::text NOT NULL,
    terms text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    signed_document_id bigint,
    created_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: contracts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.contracts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: contracts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.contracts_id_seq OWNED BY public.contracts.id;


--
-- Name: cost_center_secondary_owners; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cost_center_secondary_owners (
    cost_center_id bigint NOT NULL,
    user_id bigint NOT NULL
);


--
-- Name: cost_centers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cost_centers (
    id bigint NOT NULL,
    code text DEFAULT ''::text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    primary_owner_id bigint,
    budget numeric(16,2) DEFAULT 0 NOT NULL,
    currency text DEFAULT ''::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: cost_centers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.cost_centers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: cost_centers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.cost_centers_id_seq OWNED BY public.cost_centers.id;


--
-- Name: documents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.documents (
    id bigint NOT NULL,
    purchase_request_id bigint NOT NULL,
    filename text NOT NULL,
    stored_path text NOT NULL,
    content_type text DEFAULT 'application/octet-stream'::text NOT NULL,
    size_bytes bigint DEFAULT 0 NOT NULL,
    uploaded_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    owner_type text DEFAULT 'purchase_request'::text NOT NULL,
    owner_id bigint,
    notes text DEFAULT ''::text NOT NULL
);


--
-- Name: documents_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.documents_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: documents_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.documents_id_seq OWNED BY public.documents.id;


--
-- Name: grn_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.grn_items (
    id bigint NOT NULL,
    grn_id bigint NOT NULL,
    description text NOT NULL,
    quantity numeric(14,3) DEFAULT 1 NOT NULL,
    "position" integer DEFAULT 0 NOT NULL
);


--
-- Name: grn_items_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.grn_items_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: grn_items_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.grn_items_id_seq OWNED BY public.grn_items.id;


--
-- Name: grns; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.grns (
    id bigint NOT NULL,
    contract_id bigint NOT NULL,
    purchase_request_id bigint NOT NULL,
    vendor_id bigint NOT NULL,
    received_date date NOT NULL,
    received_by text DEFAULT ''::text NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    created_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: grns_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.grns_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: grns_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.grns_id_seq OWNED BY public.grns.id;


--
-- Name: invoice_cost_allocations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.invoice_cost_allocations (
    id bigint NOT NULL,
    invoice_id bigint NOT NULL,
    cost_center_id bigint NOT NULL,
    value numeric(16,4) DEFAULT 0 NOT NULL,
    "position" integer DEFAULT 0 NOT NULL
);


--
-- Name: invoice_cost_allocations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.invoice_cost_allocations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: invoice_cost_allocations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.invoice_cost_allocations_id_seq OWNED BY public.invoice_cost_allocations.id;


--
-- Name: invoice_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.invoice_items (
    id bigint NOT NULL,
    invoice_id bigint NOT NULL,
    description text NOT NULL,
    quantity numeric(14,3) DEFAULT 1 NOT NULL,
    unit_price numeric(16,2) DEFAULT 0 NOT NULL,
    "position" integer DEFAULT 0 NOT NULL
);


--
-- Name: invoice_items_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.invoice_items_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: invoice_items_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.invoice_items_id_seq OWNED BY public.invoice_items.id;


--
-- Name: invoices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.invoices (
    id bigint NOT NULL,
    contract_id bigint NOT NULL,
    purchase_request_id bigint NOT NULL,
    vendor_id bigint NOT NULL,
    vendor_invoice_no text DEFAULT ''::text NOT NULL,
    invoice_date date NOT NULL,
    due_date date,
    total_amount numeric(16,2) DEFAULT 0 NOT NULL,
    currency text DEFAULT 'USD'::text NOT NULL,
    status text DEFAULT 'received'::text NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    approved_by bigint,
    approved_at timestamp with time zone,
    paid_date date,
    created_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    allocation_mode text DEFAULT 'percentage'::text NOT NULL,
    entered_total numeric(16,2)
);


--
-- Name: invoices_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.invoices_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: invoices_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.invoices_id_seq OWNED BY public.invoices.id;


--
-- Name: pr_approvals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pr_approvals (
    id bigint NOT NULL,
    purchase_request_id bigint NOT NULL,
    approver_id bigint NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    comment text DEFAULT ''::text NOT NULL,
    decided_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: pr_approvals_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.pr_approvals_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: pr_approvals_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.pr_approvals_id_seq OWNED BY public.pr_approvals.id;


--
-- Name: pr_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pr_items (
    id bigint NOT NULL,
    purchase_request_id bigint NOT NULL,
    description text NOT NULL,
    quantity numeric(14,3) DEFAULT 1 NOT NULL,
    "position" integer DEFAULT 0 NOT NULL
);


--
-- Name: pr_items_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.pr_items_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: pr_items_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.pr_items_id_seq OWNED BY public.pr_items.id;


--
-- Name: pr_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pr_links (
    id bigint NOT NULL,
    purchase_request_id bigint NOT NULL,
    url text NOT NULL,
    label text DEFAULT ''::text NOT NULL,
    "position" integer DEFAULT 0 NOT NULL
);


--
-- Name: pr_links_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.pr_links_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: pr_links_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.pr_links_id_seq OWNED BY public.pr_links.id;


--
-- Name: pr_recommendation_approvals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pr_recommendation_approvals (
    recommendation_id bigint NOT NULL,
    approval_type text NOT NULL,
    approved_by bigint,
    approved_at timestamp with time zone,
    assignee_id bigint
);


--
-- Name: pr_recommendation_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pr_recommendation_comments (
    id bigint NOT NULL,
    recommendation_id bigint NOT NULL,
    approval_type text NOT NULL,
    author_id bigint NOT NULL,
    comment text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: pr_recommendation_comments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.pr_recommendation_comments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: pr_recommendation_comments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.pr_recommendation_comments_id_seq OWNED BY public.pr_recommendation_comments.id;


--
-- Name: pr_recommendations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pr_recommendations (
    id bigint NOT NULL,
    purchase_request_id bigint NOT NULL,
    vendor_id bigint NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    contract_id bigint,
    rfi_description text DEFAULT ''::text NOT NULL
);


--
-- Name: pr_recommendations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.pr_recommendations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: pr_recommendations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.pr_recommendations_id_seq OWNED BY public.pr_recommendations.id;


--
-- Name: pr_reference_sequences; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pr_reference_sequences (
    year integer NOT NULL,
    last_seq bigint DEFAULT 0 NOT NULL
);


--
-- Name: process_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.process_events (
    id bigint NOT NULL,
    purchase_request_id bigint NOT NULL,
    action text NOT NULL,
    qualifier text DEFAULT ''::text NOT NULL,
    actor_id bigint,
    actor_email text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: process_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.process_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: process_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.process_events_id_seq OWNED BY public.process_events.id;


--
-- Name: purchase_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.purchase_requests (
    id bigint NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    requester_id bigint NOT NULL,
    cost_center text DEFAULT ''::text NOT NULL,
    comments text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'submitted'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    rejection_reason text DEFAULT ''::text NOT NULL,
    cost_center_id bigint,
    team text DEFAULT ''::text NOT NULL,
    entity text DEFAULT ''::text NOT NULL,
    category text DEFAULT ''::text NOT NULL,
    estimated_value numeric(16,2) DEFAULT 0 NOT NULL,
    currency text DEFAULT ''::text NOT NULL,
    budget_approver_name text DEFAULT ''::text NOT NULL,
    budget_approver_email text DEFAULT ''::text NOT NULL,
    details jsonb DEFAULT '{}'::jsonb NOT NULL,
    reference text,
    team_lead_email text DEFAULT ''::text NOT NULL,
    team_lead_status text DEFAULT 'pending'::text NOT NULL,
    team_lead_notes text DEFAULT ''::text NOT NULL,
    team_lead_decided_at timestamp with time zone,
    team_lead_decided_by bigint,
    CONSTRAINT purchase_requests_team_lead_status_check CHECK ((team_lead_status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text])))
);


--
-- Name: purchase_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.purchase_requests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: purchase_requests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.purchase_requests_id_seq OWNED BY public.purchase_requests.id;


--
-- Name: quotation_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.quotation_items (
    id bigint NOT NULL,
    quotation_id bigint NOT NULL,
    description text NOT NULL,
    quantity numeric(14,3) DEFAULT 1 NOT NULL,
    unit_price numeric(16,2) DEFAULT 0 NOT NULL,
    "position" integer DEFAULT 0 NOT NULL
);


--
-- Name: quotation_items_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.quotation_items_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: quotation_items_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.quotation_items_id_seq OWNED BY public.quotation_items.id;


--
-- Name: quotations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.quotations (
    id bigint NOT NULL,
    vendor_id bigint NOT NULL,
    total_amount numeric(16,2) DEFAULT 0 NOT NULL,
    currency text DEFAULT 'USD'::text NOT NULL,
    valid_until date,
    notes text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'received'::text NOT NULL,
    created_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    purchase_request_id bigint NOT NULL,
    quotation_document_id bigint
);


--
-- Name: quotations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.quotations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: quotations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.quotations_id_seq OWNED BY public.quotations.id;


--
-- Name: roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.roles (
    id bigint NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: roles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.roles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: roles_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.roles_id_seq OWNED BY public.roles.id;


--
-- Name: storage_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.storage_settings (
    id smallint DEFAULT 1 NOT NULL,
    backend text DEFAULT 'gdrive'::text NOT NULL,
    base_folder_id text DEFAULT ''::text NOT NULL,
    base_folder_name text DEFAULT ''::text NOT NULL,
    google_account_email text DEFAULT ''::text NOT NULL,
    refresh_token_enc text DEFAULT ''::text NOT NULL,
    updated_by bigint,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_settings_id_check CHECK ((id = 1))
);


--
-- Name: teams; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.teams (
    id bigint NOT NULL,
    key text NOT NULL,
    name text NOT NULL,
    member_role text NOT NULL,
    team_email text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: teams_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.teams_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: teams_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.teams_id_seq OWNED BY public.teams.id;


--
-- Name: user_roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_roles (
    user_id bigint NOT NULL,
    role_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    sub text,
    email text,
    name text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_active boolean DEFAULT true NOT NULL
);


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: vendors; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vendors (
    id bigint NOT NULL,
    name text NOT NULL,
    contact_name text DEFAULT ''::text NOT NULL,
    email text DEFAULT ''::text NOT NULL,
    phone text DEFAULT ''::text NOT NULL,
    notes text DEFAULT ''::text NOT NULL,
    created_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    tax_id text DEFAULT ''::text NOT NULL,
    address_line text DEFAULT ''::text NOT NULL,
    city text DEFAULT ''::text NOT NULL,
    postal_code text DEFAULT ''::text NOT NULL,
    country text DEFAULT ''::text NOT NULL,
    registered boolean DEFAULT false NOT NULL,
    website text DEFAULT ''::text NOT NULL
);


--
-- Name: vendors_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.vendors_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: vendors_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.vendors_id_seq OWNED BY public.vendors.id;


--
-- Name: audit_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events ALTER COLUMN id SET DEFAULT nextval('public.audit_events_id_seq'::regclass);


--
-- Name: config_options id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.config_options ALTER COLUMN id SET DEFAULT nextval('public.config_options_id_seq'::regclass);


--
-- Name: contracts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contracts ALTER COLUMN id SET DEFAULT nextval('public.contracts_id_seq'::regclass);


--
-- Name: cost_centers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_centers ALTER COLUMN id SET DEFAULT nextval('public.cost_centers_id_seq'::regclass);


--
-- Name: documents id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents ALTER COLUMN id SET DEFAULT nextval('public.documents_id_seq'::regclass);


--
-- Name: grn_items id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn_items ALTER COLUMN id SET DEFAULT nextval('public.grn_items_id_seq'::regclass);


--
-- Name: grns id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grns ALTER COLUMN id SET DEFAULT nextval('public.grns_id_seq'::regclass);


--
-- Name: invoice_cost_allocations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoice_cost_allocations ALTER COLUMN id SET DEFAULT nextval('public.invoice_cost_allocations_id_seq'::regclass);


--
-- Name: invoice_items id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoice_items ALTER COLUMN id SET DEFAULT nextval('public.invoice_items_id_seq'::regclass);


--
-- Name: invoices id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoices ALTER COLUMN id SET DEFAULT nextval('public.invoices_id_seq'::regclass);


--
-- Name: pr_approvals id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_approvals ALTER COLUMN id SET DEFAULT nextval('public.pr_approvals_id_seq'::regclass);


--
-- Name: pr_items id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_items ALTER COLUMN id SET DEFAULT nextval('public.pr_items_id_seq'::regclass);


--
-- Name: pr_links id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_links ALTER COLUMN id SET DEFAULT nextval('public.pr_links_id_seq'::regclass);


--
-- Name: pr_recommendation_comments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendation_comments ALTER COLUMN id SET DEFAULT nextval('public.pr_recommendation_comments_id_seq'::regclass);


--
-- Name: pr_recommendations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendations ALTER COLUMN id SET DEFAULT nextval('public.pr_recommendations_id_seq'::regclass);


--
-- Name: process_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.process_events ALTER COLUMN id SET DEFAULT nextval('public.process_events_id_seq'::regclass);


--
-- Name: purchase_requests id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_requests ALTER COLUMN id SET DEFAULT nextval('public.purchase_requests_id_seq'::regclass);


--
-- Name: quotation_items id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quotation_items ALTER COLUMN id SET DEFAULT nextval('public.quotation_items_id_seq'::regclass);


--
-- Name: quotations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quotations ALTER COLUMN id SET DEFAULT nextval('public.quotations_id_seq'::regclass);


--
-- Name: roles id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles ALTER COLUMN id SET DEFAULT nextval('public.roles_id_seq'::regclass);


--
-- Name: teams id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams ALTER COLUMN id SET DEFAULT nextval('public.teams_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Name: vendors id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendors ALTER COLUMN id SET DEFAULT nextval('public.vendors_id_seq'::regclass);


--
-- Name: audit_events audit_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_pkey PRIMARY KEY (id);


--
-- Name: config_options config_options_list_key_value_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.config_options
    ADD CONSTRAINT config_options_list_key_value_key UNIQUE (list_key, value);


--
-- Name: config_options config_options_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.config_options
    ADD CONSTRAINT config_options_pkey PRIMARY KEY (id);


--
-- Name: contracts contracts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contracts
    ADD CONSTRAINT contracts_pkey PRIMARY KEY (id);


--
-- Name: cost_center_secondary_owners cost_center_secondary_owners_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_center_secondary_owners
    ADD CONSTRAINT cost_center_secondary_owners_pkey PRIMARY KEY (cost_center_id, user_id);


--
-- Name: cost_centers cost_centers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_centers
    ADD CONSTRAINT cost_centers_pkey PRIMARY KEY (id);


--
-- Name: documents documents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_pkey PRIMARY KEY (id);


--
-- Name: grn_items grn_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn_items
    ADD CONSTRAINT grn_items_pkey PRIMARY KEY (id);


--
-- Name: grns grns_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grns
    ADD CONSTRAINT grns_pkey PRIMARY KEY (id);


--
-- Name: invoice_cost_allocations invoice_cost_allocations_invoice_id_cost_center_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoice_cost_allocations
    ADD CONSTRAINT invoice_cost_allocations_invoice_id_cost_center_id_key UNIQUE (invoice_id, cost_center_id);


--
-- Name: invoice_cost_allocations invoice_cost_allocations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoice_cost_allocations
    ADD CONSTRAINT invoice_cost_allocations_pkey PRIMARY KEY (id);


--
-- Name: invoice_items invoice_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoice_items
    ADD CONSTRAINT invoice_items_pkey PRIMARY KEY (id);


--
-- Name: invoices invoices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT invoices_pkey PRIMARY KEY (id);


--
-- Name: pr_approvals pr_approvals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_approvals
    ADD CONSTRAINT pr_approvals_pkey PRIMARY KEY (id);


--
-- Name: pr_approvals pr_approvals_purchase_request_id_approver_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_approvals
    ADD CONSTRAINT pr_approvals_purchase_request_id_approver_id_key UNIQUE (purchase_request_id, approver_id);


--
-- Name: pr_items pr_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_items
    ADD CONSTRAINT pr_items_pkey PRIMARY KEY (id);


--
-- Name: pr_links pr_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_links
    ADD CONSTRAINT pr_links_pkey PRIMARY KEY (id);


--
-- Name: pr_recommendation_approvals pr_recommendation_approvals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendation_approvals
    ADD CONSTRAINT pr_recommendation_approvals_pkey PRIMARY KEY (recommendation_id, approval_type);


--
-- Name: pr_recommendation_comments pr_recommendation_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendation_comments
    ADD CONSTRAINT pr_recommendation_comments_pkey PRIMARY KEY (id);


--
-- Name: pr_recommendations pr_recommendations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendations
    ADD CONSTRAINT pr_recommendations_pkey PRIMARY KEY (id);


--
-- Name: pr_recommendations pr_recommendations_purchase_request_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendations
    ADD CONSTRAINT pr_recommendations_purchase_request_id_key UNIQUE (purchase_request_id);


--
-- Name: pr_reference_sequences pr_reference_sequences_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_reference_sequences
    ADD CONSTRAINT pr_reference_sequences_pkey PRIMARY KEY (year);


--
-- Name: process_events process_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.process_events
    ADD CONSTRAINT process_events_pkey PRIMARY KEY (id);


--
-- Name: purchase_requests purchase_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_requests
    ADD CONSTRAINT purchase_requests_pkey PRIMARY KEY (id);


--
-- Name: quotation_items quotation_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quotation_items
    ADD CONSTRAINT quotation_items_pkey PRIMARY KEY (id);


--
-- Name: quotations quotations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quotations
    ADD CONSTRAINT quotations_pkey PRIMARY KEY (id);


--
-- Name: roles roles_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_name_key UNIQUE (name);


--
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);


--
-- Name: storage_settings storage_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.storage_settings
    ADD CONSTRAINT storage_settings_pkey PRIMARY KEY (id);


--
-- Name: teams teams_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_key_key UNIQUE (key);


--
-- Name: teams teams_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_pkey PRIMARY KEY (id);


--
-- Name: user_roles user_roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_pkey PRIMARY KEY (user_id, role_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users users_sub_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_sub_key UNIQUE (sub);


--
-- Name: vendors vendors_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendors
    ADD CONSTRAINT vendors_pkey PRIMARY KEY (id);


--
-- Name: idx_audit_events_action; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_events_action ON public.audit_events USING btree (action, created_at);


--
-- Name: idx_audit_events_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_events_entity ON public.audit_events USING btree (entity_type, entity_id);


--
-- Name: idx_config_options_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_config_options_key ON public.config_options USING btree (list_key, sort_order);


--
-- Name: idx_contracts_pr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contracts_pr ON public.contracts USING btree (purchase_request_id);


--
-- Name: idx_contracts_quotation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contracts_quotation ON public.contracts USING btree (quotation_id);


--
-- Name: idx_cost_centers_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_cost_centers_code ON public.cost_centers USING btree (code) WHERE (code <> ''::text);


--
-- Name: idx_cost_centers_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cost_centers_name ON public.cost_centers USING btree (name);


--
-- Name: idx_documents_owner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_documents_owner ON public.documents USING btree (owner_type, owner_id);


--
-- Name: idx_documents_pr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_documents_pr ON public.documents USING btree (purchase_request_id);


--
-- Name: idx_grn_items_grn; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_grn_items_grn ON public.grn_items USING btree (grn_id);


--
-- Name: idx_grns_contract; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_grns_contract ON public.grns USING btree (contract_id);


--
-- Name: idx_grns_pr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_grns_pr ON public.grns USING btree (purchase_request_id);


--
-- Name: idx_invoice_cost_allocations_cost_center; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_invoice_cost_allocations_cost_center ON public.invoice_cost_allocations USING btree (cost_center_id);


--
-- Name: idx_invoice_cost_allocations_invoice; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_invoice_cost_allocations_invoice ON public.invoice_cost_allocations USING btree (invoice_id);


--
-- Name: idx_invoice_items_invoice; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_invoice_items_invoice ON public.invoice_items USING btree (invoice_id);


--
-- Name: idx_invoices_contract; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_invoices_contract ON public.invoices USING btree (contract_id);


--
-- Name: idx_invoices_pr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_invoices_pr ON public.invoices USING btree (purchase_request_id);


--
-- Name: idx_pr_approvals_approver; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pr_approvals_approver ON public.pr_approvals USING btree (approver_id);


--
-- Name: idx_pr_approvals_pr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pr_approvals_pr ON public.pr_approvals USING btree (purchase_request_id);


--
-- Name: idx_pr_items_pr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pr_items_pr ON public.pr_items USING btree (purchase_request_id);


--
-- Name: idx_pr_links_pr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pr_links_pr ON public.pr_links USING btree (purchase_request_id);


--
-- Name: idx_pr_rec_comments; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pr_rec_comments ON public.pr_recommendation_comments USING btree (recommendation_id, approval_type);


--
-- Name: idx_pr_requester; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pr_requester ON public.purchase_requests USING btree (requester_id);


--
-- Name: idx_pr_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pr_status ON public.purchase_requests USING btree (status);


--
-- Name: idx_process_events_action; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_process_events_action ON public.process_events USING btree (action);


--
-- Name: idx_process_events_pr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_process_events_pr ON public.process_events USING btree (purchase_request_id, created_at);


--
-- Name: idx_purchase_requests_reference; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_purchase_requests_reference ON public.purchase_requests USING btree (reference);


--
-- Name: idx_purchase_requests_team_lead_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_purchase_requests_team_lead_email ON public.purchase_requests USING btree (team_lead_email);


--
-- Name: idx_quotation_items_q; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quotation_items_q ON public.quotation_items USING btree (quotation_id);


--
-- Name: idx_quotations_pr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quotations_pr ON public.quotations USING btree (purchase_request_id);


--
-- Name: idx_quotations_vendor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quotations_vendor ON public.quotations USING btree (vendor_id);


--
-- Name: idx_vendors_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vendors_active ON public.vendors USING btree (is_active);


--
-- Name: idx_vendors_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vendors_name ON public.vendors USING btree (name);


--
-- Name: users_email_lower_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX users_email_lower_key ON public.users USING btree (lower(email)) WHERE (email IS NOT NULL);


--
-- Name: audit_events audit_events_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: contracts contracts_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contracts
    ADD CONSTRAINT contracts_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: contracts contracts_purchase_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contracts
    ADD CONSTRAINT contracts_purchase_request_id_fkey FOREIGN KEY (purchase_request_id) REFERENCES public.purchase_requests(id) ON DELETE CASCADE;


--
-- Name: contracts contracts_quotation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contracts
    ADD CONSTRAINT contracts_quotation_id_fkey FOREIGN KEY (quotation_id) REFERENCES public.quotations(id) ON DELETE SET NULL;


--
-- Name: contracts contracts_signed_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contracts
    ADD CONSTRAINT contracts_signed_document_id_fkey FOREIGN KEY (signed_document_id) REFERENCES public.documents(id) ON DELETE SET NULL;


--
-- Name: contracts contracts_vendor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contracts
    ADD CONSTRAINT contracts_vendor_id_fkey FOREIGN KEY (vendor_id) REFERENCES public.vendors(id) ON DELETE RESTRICT;


--
-- Name: cost_center_secondary_owners cost_center_secondary_owners_cost_center_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_center_secondary_owners
    ADD CONSTRAINT cost_center_secondary_owners_cost_center_id_fkey FOREIGN KEY (cost_center_id) REFERENCES public.cost_centers(id) ON DELETE CASCADE;


--
-- Name: cost_center_secondary_owners cost_center_secondary_owners_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_center_secondary_owners
    ADD CONSTRAINT cost_center_secondary_owners_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: cost_centers cost_centers_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_centers
    ADD CONSTRAINT cost_centers_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: cost_centers cost_centers_primary_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_centers
    ADD CONSTRAINT cost_centers_primary_owner_id_fkey FOREIGN KEY (primary_owner_id) REFERENCES public.users(id);


--
-- Name: documents documents_purchase_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_purchase_request_id_fkey FOREIGN KEY (purchase_request_id) REFERENCES public.purchase_requests(id) ON DELETE CASCADE;


--
-- Name: documents documents_uploaded_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_uploaded_by_fkey FOREIGN KEY (uploaded_by) REFERENCES public.users(id);


--
-- Name: grn_items grn_items_grn_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn_items
    ADD CONSTRAINT grn_items_grn_id_fkey FOREIGN KEY (grn_id) REFERENCES public.grns(id) ON DELETE CASCADE;


--
-- Name: grns grns_contract_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grns
    ADD CONSTRAINT grns_contract_id_fkey FOREIGN KEY (contract_id) REFERENCES public.contracts(id) ON DELETE CASCADE;


--
-- Name: grns grns_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grns
    ADD CONSTRAINT grns_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: grns grns_purchase_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grns
    ADD CONSTRAINT grns_purchase_request_id_fkey FOREIGN KEY (purchase_request_id) REFERENCES public.purchase_requests(id) ON DELETE CASCADE;


--
-- Name: grns grns_vendor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grns
    ADD CONSTRAINT grns_vendor_id_fkey FOREIGN KEY (vendor_id) REFERENCES public.vendors(id) ON DELETE RESTRICT;


--
-- Name: invoice_cost_allocations invoice_cost_allocations_cost_center_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoice_cost_allocations
    ADD CONSTRAINT invoice_cost_allocations_cost_center_id_fkey FOREIGN KEY (cost_center_id) REFERENCES public.cost_centers(id);


--
-- Name: invoice_cost_allocations invoice_cost_allocations_invoice_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoice_cost_allocations
    ADD CONSTRAINT invoice_cost_allocations_invoice_id_fkey FOREIGN KEY (invoice_id) REFERENCES public.invoices(id) ON DELETE CASCADE;


--
-- Name: invoice_items invoice_items_invoice_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoice_items
    ADD CONSTRAINT invoice_items_invoice_id_fkey FOREIGN KEY (invoice_id) REFERENCES public.invoices(id) ON DELETE CASCADE;


--
-- Name: invoices invoices_approved_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT invoices_approved_by_fkey FOREIGN KEY (approved_by) REFERENCES public.users(id);


--
-- Name: invoices invoices_contract_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT invoices_contract_id_fkey FOREIGN KEY (contract_id) REFERENCES public.contracts(id) ON DELETE CASCADE;


--
-- Name: invoices invoices_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT invoices_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: invoices invoices_purchase_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT invoices_purchase_request_id_fkey FOREIGN KEY (purchase_request_id) REFERENCES public.purchase_requests(id) ON DELETE CASCADE;


--
-- Name: invoices invoices_vendor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT invoices_vendor_id_fkey FOREIGN KEY (vendor_id) REFERENCES public.vendors(id) ON DELETE RESTRICT;


--
-- Name: pr_approvals pr_approvals_approver_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_approvals
    ADD CONSTRAINT pr_approvals_approver_id_fkey FOREIGN KEY (approver_id) REFERENCES public.users(id);


--
-- Name: pr_approvals pr_approvals_purchase_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_approvals
    ADD CONSTRAINT pr_approvals_purchase_request_id_fkey FOREIGN KEY (purchase_request_id) REFERENCES public.purchase_requests(id) ON DELETE CASCADE;


--
-- Name: pr_items pr_items_purchase_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_items
    ADD CONSTRAINT pr_items_purchase_request_id_fkey FOREIGN KEY (purchase_request_id) REFERENCES public.purchase_requests(id) ON DELETE CASCADE;


--
-- Name: pr_links pr_links_purchase_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_links
    ADD CONSTRAINT pr_links_purchase_request_id_fkey FOREIGN KEY (purchase_request_id) REFERENCES public.purchase_requests(id) ON DELETE CASCADE;


--
-- Name: pr_recommendation_approvals pr_recommendation_approvals_approved_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendation_approvals
    ADD CONSTRAINT pr_recommendation_approvals_approved_by_fkey FOREIGN KEY (approved_by) REFERENCES public.users(id);


--
-- Name: pr_recommendation_approvals pr_recommendation_approvals_assignee_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendation_approvals
    ADD CONSTRAINT pr_recommendation_approvals_assignee_id_fkey FOREIGN KEY (assignee_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: pr_recommendation_approvals pr_recommendation_approvals_recommendation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendation_approvals
    ADD CONSTRAINT pr_recommendation_approvals_recommendation_id_fkey FOREIGN KEY (recommendation_id) REFERENCES public.pr_recommendations(id) ON DELETE CASCADE;


--
-- Name: pr_recommendation_comments pr_recommendation_comments_author_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendation_comments
    ADD CONSTRAINT pr_recommendation_comments_author_id_fkey FOREIGN KEY (author_id) REFERENCES public.users(id);


--
-- Name: pr_recommendation_comments pr_recommendation_comments_recommendation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendation_comments
    ADD CONSTRAINT pr_recommendation_comments_recommendation_id_fkey FOREIGN KEY (recommendation_id) REFERENCES public.pr_recommendations(id) ON DELETE CASCADE;


--
-- Name: pr_recommendations pr_recommendations_contract_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendations
    ADD CONSTRAINT pr_recommendations_contract_id_fkey FOREIGN KEY (contract_id) REFERENCES public.contracts(id) ON DELETE SET NULL;


--
-- Name: pr_recommendations pr_recommendations_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendations
    ADD CONSTRAINT pr_recommendations_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: pr_recommendations pr_recommendations_purchase_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendations
    ADD CONSTRAINT pr_recommendations_purchase_request_id_fkey FOREIGN KEY (purchase_request_id) REFERENCES public.purchase_requests(id) ON DELETE CASCADE;


--
-- Name: pr_recommendations pr_recommendations_vendor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_recommendations
    ADD CONSTRAINT pr_recommendations_vendor_id_fkey FOREIGN KEY (vendor_id) REFERENCES public.vendors(id) ON DELETE RESTRICT;


--
-- Name: process_events process_events_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.process_events
    ADD CONSTRAINT process_events_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: process_events process_events_purchase_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.process_events
    ADD CONSTRAINT process_events_purchase_request_id_fkey FOREIGN KEY (purchase_request_id) REFERENCES public.purchase_requests(id) ON DELETE CASCADE;


--
-- Name: purchase_requests purchase_requests_cost_center_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_requests
    ADD CONSTRAINT purchase_requests_cost_center_id_fkey FOREIGN KEY (cost_center_id) REFERENCES public.cost_centers(id);


--
-- Name: purchase_requests purchase_requests_requester_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_requests
    ADD CONSTRAINT purchase_requests_requester_id_fkey FOREIGN KEY (requester_id) REFERENCES public.users(id);


--
-- Name: purchase_requests purchase_requests_team_lead_decided_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_requests
    ADD CONSTRAINT purchase_requests_team_lead_decided_by_fkey FOREIGN KEY (team_lead_decided_by) REFERENCES public.users(id);


--
-- Name: quotation_items quotation_items_quotation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quotation_items
    ADD CONSTRAINT quotation_items_quotation_id_fkey FOREIGN KEY (quotation_id) REFERENCES public.quotations(id) ON DELETE CASCADE;


--
-- Name: quotations quotations_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quotations
    ADD CONSTRAINT quotations_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: quotations quotations_purchase_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quotations
    ADD CONSTRAINT quotations_purchase_request_id_fkey FOREIGN KEY (purchase_request_id) REFERENCES public.purchase_requests(id) ON DELETE CASCADE;


--
-- Name: quotations quotations_quotation_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quotations
    ADD CONSTRAINT quotations_quotation_document_id_fkey FOREIGN KEY (quotation_document_id) REFERENCES public.documents(id) ON DELETE SET NULL;


--
-- Name: quotations quotations_vendor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quotations
    ADD CONSTRAINT quotations_vendor_id_fkey FOREIGN KEY (vendor_id) REFERENCES public.vendors(id) ON DELETE RESTRICT;


--
-- Name: storage_settings storage_settings_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.storage_settings
    ADD CONSTRAINT storage_settings_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: teams teams_member_role_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_member_role_fkey FOREIGN KEY (member_role) REFERENCES public.roles(name);


--
-- Name: user_roles user_roles_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- Name: user_roles user_roles_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: vendors vendors_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendors
    ADD CONSTRAINT vendors_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- PostgreSQL database dump complete
--

\unrestrict rmtS88jEiFfF9p0g3Xt3X2hRSQcqGbg6UkWmKqN9MdbZN5D75ItKcpTvDyZXYog

