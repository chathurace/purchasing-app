import { useState } from "react";
import { useVendorLookup } from "../hooks/useVendors";
import { useBusinessUnitApprovers, useBusinessUnitLookup } from "../hooks/useBusinessUnits";
import { useDirectory } from "../hooks/useDirectory";
import { EmailAutocomplete } from "./EmailAutocomplete";
import { SupplierNameCombobox } from "./SupplierNameCombobox";
import type {
  PRCategory,
  PRDetails,
  PurchaseRequestInput,
  VendorLookup,
  YesNo,
} from "../types/api";
import "./RequisitionForm.css";

// isEmail is a lightweight sanity check for email inputs — the server is the
// source of truth.
const isEmail = (s: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(s.trim());

const USAGE_PERIODS = [
  "Trial / pilot (under 3 months)",
  "Monthly plan",
  "1 year",
  "2 years",
  "3+ years",
  "Ongoing / perpetual",
  "Other",
];

const INTEGRATION_KINDS = ["API integration", "SSO / identity", "Data sync / ETL", "Webhook", "Other"];

// Step titles. Step 3's label is category-dependent (filled in at render).
const STEP_TITLES = ["Requester details", "Procurement Category", "Requirement Details", "Vendor & budget", "Review & submit"];

const CATEGORY_LABEL: Record<Exclude<PRCategory, "">, string> = {
  IT: "IT Related Procurement",
  "NON-IT": "Non-IT Related Procurement",
  EVENTS: "Marketing & Events Procurement",
};

const DETAIL_LABEL: Record<Exclude<PRCategory, "">, string> = {
  IT: "IT Requirement Details",
  "NON-IT": "Non-IT Requirement Details",
  EVENTS: "Event Requirement Details",
};

interface Props {
  value: PurchaseRequestInput;
  onChange: (next: PurchaseRequestInput) => void;
  onSubmit: () => void;
  submitting: boolean;
  submitLabel: string;
  requireDeclaration?: boolean;
  error?: string | null;
  // Supplier attachments staged during create; when provided, the file field is
  // shown and the host page uploads them after the PR is created. Omitted in edit
  // mode (the detail page's Documents card manages documents there).
  attachments?: File[];
  onAttachmentsChange?: (files: File[]) => void;
}

export function RequisitionForm({
  value,
  onChange,
  onSubmit,
  submitting,
  submitLabel,
  requireDeclaration = false,
  error,
  attachments,
  onAttachmentsChange,
}: Props) {
  const [step, setStep] = useState(1);
  const [declared, setDeclared] = useState(false);
  const [vErr, setVErr] = useState<string | null>(null);

  const d: PRDetails = value.details ?? {};
  const cat = value.category;
  const setTop = (patch: Partial<PurchaseRequestInput>) => onChange({ ...value, ...patch });
  const setD = (patch: Partial<PRDetails>) => onChange({ ...value, details: { ...d, ...patch } });

  const goTo = (n: number) => {
    setVErr(null);
    setStep(n);
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  const nonEmpty = (arr?: string[]) => (arr ?? []).map((s) => s.trim()).filter(Boolean);

  // Returns the labels of required fields missing on a given step.
  const missingOn = (n: number): string[] => {
    const need: [boolean, string][] = [];
    if (n === 1) {
      need.push(
        [!d.date, "Date"],
        [!d.requester_name?.trim(), "Your full name"],
        [!d.requester_email?.trim(), "WSO2 email"],
        [!value.team_lead_email?.trim(), "Team lead email"],
        [!!value.team_lead_email?.trim() && !isEmail(value.team_lead_email), "a valid team lead email"],
      );
    } else if (n === 2) {
      need.push([!cat, "Procurement category"]);
    } else if (n === 3) {
      if (cat === "IT") {
        need.push(
          [!d.it_product?.trim(), "Product / solution name"],
          [!d.it_plan?.trim(), "Plan / tier"],
          [!d.it_description?.trim(), "Description"],
          [!d.it_user_count?.trim(), "Number of users"],
          [nonEmpty(d.it_user_names).length === 0, "Names of users / teams"],
          [!d.it_admin_count?.trim(), "Number of administrators"],
          [nonEmpty(d.it_admin_names).length === 0, "Names of administrators"],
          [!d.it_usage, "Expected usage period"],
          [!d.business_justification?.trim(), "Business justification"],
          [!d.sec_sensitive, "Sensitive data answer"],
          [!d.sec_external_pii, "External PII answer"],
          [d.sec_external_pii === "yes" && !d.sec_external_pii_detail?.trim(), "External PII detail"],
          [!d.sec_employee_pii, "Employee PII answer"],
          [d.sec_employee_pii === "yes" && !d.sec_employee_pii_detail?.trim(), "Employee PII detail"],
          [!d.sec_integrates, "Integration answer"],
          [d.sec_integrates === "yes" && !d.sec_integration_systems?.trim(), "Integration systems"],
          [d.sec_integrates === "yes" && !d.sec_integration_kind, "Integration type"],
          [d.sec_integrates === "yes" && !d.sec_vendor_docs, "Vendor documentation answer"],
          [d.sec_integrates === "yes" && d.sec_vendor_docs === "yes" && !d.sec_vendor_docs_link?.trim(), "Documentation link"],
        );
      } else if (cat === "NON-IT") {
        need.push([!d.nit_description?.trim(), "Details of goods / services"], [!d.business_justification?.trim(), "Business justification"]);
      }
      // EVENTS: under development — nothing to validate.
    } else if (n === 4) {
      if (cat === "IT" || cat === "NON-IT") {
        need.push(
          [!value.business_unit_id, "Business unit"],
          [!value.budget_approver_email?.trim(), "Budget approver"],
        );
      }
      // EVENTS: under development — nothing to validate.
    }
    return need.filter(([bad]) => bad).map(([, label]) => label);
  };

  const next = () => {
    const missing = missingOn(step);
    if (missing.length) {
      setVErr("Please complete: " + missing.join(", "));
      return;
    }
    goTo(step + 1);
  };

  const submit = () => {
    for (let n = 1; n <= 4; n++) {
      const missing = missingOn(n);
      if (missing.length) {
        setStep(n);
        setVErr("Please complete: " + missing.join(", "));
        return;
      }
    }
    if (requireDeclaration && !declared) {
      setStep(5);
      setVErr("Please confirm the declaration before submitting.");
      return;
    }
    setVErr(null);
    onSubmit();
  };

  const detailLabel = cat ? DETAIL_LABEL[cat] : "Requirement Details";

  return (
    <div className="proq-req">
      <Hero step={step} detailLabel={detailLabel} onStep={goTo} />

      <div className="form-card">
        {step === 1 && <StepRequester value={value} d={d} setTop={setTop} setD={setD} />}
        {step === 2 && <StepCategory cat={cat} setTop={setTop} />}
        {step === 3 && <StepRequirement cat={cat} d={d} setD={setD} setTop={setTop} detailLabel={detailLabel} />}
        {step === 4 && (
          <StepVendorBudget
            cat={cat}
            value={value}
            d={d}
            setTop={setTop}
            setD={setD}
            attachments={attachments}
            onAttachmentsChange={onAttachmentsChange}
          />
        )}
        {step === 5 && (
          <StepReview
            value={value}
            d={d}
            onStep={goTo}
            requireDeclaration={requireDeclaration}
            declared={declared}
            setDeclared={setDeclared}
          />
        )}

        {(vErr || error) && <div className="error-msg">⚠ {vErr || error}</div>}

        <div className="form-actions">
          {step > 1 ? (
            <button type="button" className="btn btn-ghost" onClick={() => goTo(step - 1)}>
              ← Back
            </button>
          ) : (
            <span />
          )}
          {step < 5 ? (
            <button type="button" className="btn btn-primary" onClick={next}>
              {step === 4 ? "Next → Review" : step === 2 && cat ? `Continue to ${detailLabel} →` : "Continue →"}
            </button>
          ) : (
            <button type="button" className="btn btn-submit" onClick={submit} disabled={submitting}>
              {submitting ? "Submitting…" : `✓ ${submitLabel}`}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

// --- hero + stepper ---

function Hero({ step, detailLabel, onStep }: { step: number; detailLabel: string; onStep: (n: number) => void }) {
  const labels = [...STEP_TITLES];
  labels[2] = detailLabel;
  return (
    <div className="page-hero">
      <div className="page-hero-inner page-hero-top">
        <div className="crumb">Requests / New</div>
        <h1>New Purchase Requisition (PR)</h1>
        <p className="sub">
          This is the first step of WSO2's supply chain cycle. Complete the short steps below — once you submit, the
          WSO2 Procurement team takes over and manages your request through the full procurement lifecycle.
        </p>
      </div>
      <div className="hero-divider" />
      <div className="page-hero-inner page-hero-bottom">
        <div className="stepper" role="tablist">
          {labels.map((label, i) => {
            const n = i + 1;
            const cls = n === step ? "current" : n < step ? "done reached" : "";
            return (
              <button type="button" key={label} className={`step-tab ${cls}`} onClick={() => onStep(n)} role="tab">
                <span className="n">{n < step ? "✓" : n}</span>
                {label}
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}

// --- shared field helpers ---

function Field({ label, required, hint, children }: { label: string; required?: boolean; hint?: string; children: React.ReactNode }) {
  return (
    <div className="field">
      <label>
        {label} {required && <b>*</b>}
      </label>
      {children}
      {hint && <div className="help">{hint}</div>}
    </div>
  );
}

function SectionBand({ children }: { children: React.ReactNode }) {
  return <div className="section-band">{children}</div>;
}

function DevNote({ children }: { children: React.ReactNode }) {
  return (
    <div className="dev-note">
      <span className="icon">🚧</span>
      <span>{children}</span>
    </div>
  );
}

function YNField({ value, onChange }: { value?: YesNo; onChange: (v: YesNo) => void }) {
  return (
    <div className="yn">
      <button type="button" className={value === "yes" ? "on-yes" : ""} onClick={() => onChange("yes")}>
        Yes
      </button>
      <button type="button" className={value === "no" ? "on-no" : ""} onClick={() => onChange("no")}>
        No
      </button>
    </div>
  );
}

function Select({ value, onChange, options, placeholder }: { value?: string; onChange: (v: string) => void; options: string[]; placeholder: string }) {
  return (
    <select value={value ?? ""} onChange={(e) => onChange(e.target.value)}>
      <option value="">{placeholder}</option>
      {options.map((o) => (
        <option key={o} value={o}>
          {o}
        </option>
      ))}
    </select>
  );
}

// NameList edits a string[] of names, with a count field that pre-seeds rows.
function NameList({
  names,
  count,
  placeholder,
  onNames,
  onCount,
}: {
  names: string[];
  count: string;
  placeholder: string;
  onNames: (next: string[]) => void;
  onCount: (v: string) => void;
}) {
  const rows = names.length ? names : [""];
  const setAt = (i: number, v: string) => {
    const next = [...rows];
    next[i] = v;
    onNames(next);
  };
  const remove = (i: number) => {
    if (rows.length <= 1) {
      onNames([""]);
      return;
    }
    onNames(rows.filter((_, j) => j !== i));
  };
  const add = () => onNames([...rows, ""]);
  const syncCount = (v: string) => {
    onCount(v);
    const c = parseInt(v, 10);
    if (c > rows.length) {
      onNames([...rows, ...Array(c - rows.length).fill("")]);
    }
  };
  return (
    <>
      <input type="number" min={1} value={count} onChange={(e) => syncCount(e.target.value)} placeholder="e.g. 5" />
      <div className="name-list" style={{ marginTop: 10 }}>
        {rows.map((name, i) => (
          <div className="name-row" key={i}>
            <input type="text" value={name} onChange={(e) => setAt(i, e.target.value)} placeholder={placeholder} />
            <button type="button" className="name-remove" onClick={() => remove(i)} aria-label="Remove">
              ×
            </button>
          </div>
        ))}
      </div>
      <button type="button" className="name-add" onClick={add}>
        + Add name
      </button>
    </>
  );
}

interface DetailProps {
  cat: PRCategory;
  d: PRDetails;
  setD: (patch: Partial<PRDetails>) => void;
  setTop: (patch: Partial<PurchaseRequestInput>) => void;
}

// --- step 1 ---

function StepRequester({
  value,
  d,
  setTop,
  setD,
}: {
  value: PurchaseRequestInput;
  d: PRDetails;
  setTop: (patch: Partial<PurchaseRequestInput>) => void;
  setD: (patch: Partial<PRDetails>) => void;
}) {
  const { data: directory, isLoading: dirLoading, refresh: refreshDir } = useDirectory();
  return (
    <>
      <div className="pane-title">Requester details</div>
      <p className="pane-sub">Confirm who's raising this request. The procurement category and requirement details are captured in the next steps.</p>
      <div className="grid-3">
        <Field label="Date" required>
          <input type="date" value={d.date ?? ""} onChange={(e) => setD({ date: e.target.value })} />
        </Field>
        <Field label="Your full name" required>
          <input type="text" value={d.requester_name ?? ""} onChange={(e) => setD({ requester_name: e.target.value })} placeholder="e.g. Chamika Karunarathne" />
        </Field>
        <Field label="WSO2 email" required>
          <input type="email" value={d.requester_email ?? ""} onChange={(e) => setD({ requester_email: e.target.value })} placeholder="you@wso2.com" />
        </Field>
      </div>
      <div className="help" style={{ margin: "-8px 0 18px" }}>
        Name and email are pulled from your WSO2 SSO login — edit only if you're raising this requisition on someone else's behalf.
      </div>
      <Field label="Team lead (for approval)" required hint="Your team lead must approve this request before procurement can start working on it.">
        <EmailAutocomplete
          value={value.team_lead_email ?? ""}
          onChange={(email) => setTop({ team_lead_email: email })}
          directory={directory ?? []}
          refresh={refreshDir}
          loading={dirLoading}
          inputClassName=""
          placeholder="teamlead@wso2.com"
          ariaLabel="Team lead email"
        />
      </Field>
    </>
  );
}

// --- step 2 ---

function StepCategory({ cat, setTop }: { cat: PRCategory; setTop: (patch: Partial<PurchaseRequestInput>) => void }) {
  const card = (c: Exclude<PRCategory, "">, icon: string, title: string, desc: string) => (
    <button type="button" className={`type-card ${cat === c ? "selected" : ""}`} onClick={() => setTop({ category: c })}>
      <span className="tick">✓</span>
      <h3>
        {icon} {title}
      </h3>
      <p>{desc}</p>
    </button>
  );
  return (
    <>
      <div className="pane-title">Procurement Category</div>
      <p className="pane-sub">Choose the category that best matches your request — ProQ will then ask only the questions relevant to your selection, on the next page.</p>
      <div className="type-cards">
        {card("IT", "💻", "IT Related Procurement", "Software, SaaS subscriptions, cloud infrastructure, hardware, and other technology purchases.")}
        {card("NON-IT", "🏢", "Non-IT Related Procurement", "Facilities, insurance, office goods, consumables, and general professional services.")}
        {card("EVENTS", "🎤", "Marketing & Events Procurement", "Conferences, sponsorships, venues, AV production, and vendor services for WSO2 events and campaigns.")}
      </div>
      {!cat && <p className="type-hint">Select a procurement category above to continue.</p>}
    </>
  );
}

// --- step 3 ---

function StepRequirement({ cat, d, setD, detailLabel }: DetailProps & { detailLabel: string }) {
  return (
    <>
      <div className="pane-title">{detailLabel}</div>
      <p className="pane-sub">Complete the details below for your selected category.</p>

      {cat === "IT" && (
        <>
          <div className="grid-2">
            <Field label="Product / solution name" required>
              <input type="text" value={d.it_product ?? ""} onChange={(e) => setD({ it_product: e.target.value })} placeholder="e.g. Salesforce, GitHub Enterprise" />
            </Field>
            <Field label="Plan / subscription tier" required>
              <input type="text" value={d.it_plan ?? ""} onChange={(e) => setD({ it_plan: e.target.value })} placeholder="e.g. Enterprise, Pro" />
            </Field>
          </div>
          <Field label="Description of the product / solution" required>
            <textarea value={d.it_description ?? ""} onChange={(e) => setD({ it_description: e.target.value })} placeholder="What does it do, and how will WSO2 use it?" />
          </Field>
          <div className="grid-2">
            <Field label="Users — count &amp; names" required>
              <NameList
                names={d.it_user_names ?? []}
                count={d.it_user_count ?? ""}
                placeholder="e.g. Jane Silva — Sales team"
                onNames={(it_user_names) => setD({ it_user_names })}
                onCount={(it_user_count) => setD({ it_user_count })}
              />
            </Field>
            <Field label="Administrators — count &amp; names" required>
              <NameList
                names={d.it_admin_names ?? []}
                count={d.it_admin_count ?? ""}
                placeholder="e.g. John Perera"
                onNames={(it_admin_names) => setD({ it_admin_names })}
                onCount={(it_admin_count) => setD({ it_admin_count })}
              />
            </Field>
          </div>
          <Field label="Expected usage period" required>
            <Select value={d.it_usage} onChange={(it_usage) => setD({ it_usage })} options={USAGE_PERIODS} placeholder="Select duration" />
          </Field>
          <Field label="Business justification" required>
            <textarea
              value={d.business_justification ?? ""}
              onChange={(e) => setD({ business_justification: e.target.value })}
              placeholder="Why is this purchase needed? What problem does it solve, and what happens if we don't buy it?"
            />
          </Field>

          <SectionBand>Data &amp; security assessment</SectionBand>
          <Field label="Will this platform store or process sensitive or confidential WSO2 data?" required>
            <YNField value={d.sec_sensitive} onChange={(sec_sensitive) => setD({ sec_sensitive })} />
          </Field>
          <Field label="Will this platform store or process external PII (customers, prospects, leads, or third parties)?" required>
            <YNField value={d.sec_external_pii} onChange={(sec_external_pii) => setD({ sec_external_pii })} />
          </Field>
          {d.sec_external_pii === "yes" && (
            <div className="followup">
              <Field label="What data would this include?" required>
                <input type="text" value={d.sec_external_pii_detail ?? ""} onChange={(e) => setD({ sec_external_pii_detail: e.target.value })} placeholder="e.g. names, emails, phone numbers, payment details" />
              </Field>
            </div>
          )}
          <Field label="Will this platform store or process internal employee PII?" required>
            <YNField value={d.sec_employee_pii} onChange={(sec_employee_pii) => setD({ sec_employee_pii })} />
          </Field>
          {d.sec_employee_pii === "yes" && (
            <div className="followup">
              <Field label="What data would this include?" required>
                <input type="text" value={d.sec_employee_pii_detail ?? ""} onChange={(e) => setD({ sec_employee_pii_detail: e.target.value })} placeholder="e.g. employee ID, payroll data, performance records" />
              </Field>
            </div>
          )}
          <Field label="Will this integrate with WSO2 internal systems?" required>
            <YNField value={d.sec_integrates} onChange={(sec_integrates) => setD({ sec_integrates })} />
          </Field>
          {d.sec_integrates === "yes" && (
            <div className="followup">
              <div className="grid-2">
                <Field label="Which systems?" required>
                  <input type="text" value={d.sec_integration_systems ?? ""} onChange={(e) => setD({ sec_integration_systems: e.target.value })} placeholder="e.g. WSO2 Identity Server, Google Workspace" />
                </Field>
                <Field label="What kind of integration?" required>
                  <Select value={d.sec_integration_kind} onChange={(sec_integration_kind) => setD({ sec_integration_kind })} options={INTEGRATION_KINDS} placeholder="Select type" />
                </Field>
              </div>
              <Field label="Has the vendor provided guidelines / documentation on this integration?" required>
                <YNField value={d.sec_vendor_docs} onChange={(sec_vendor_docs) => setD({ sec_vendor_docs })} />
              </Field>
              {d.sec_vendor_docs === "yes" && (
                <div className="followup">
                  <Field label="Documentation link(s)" required>
                    <input type="text" value={d.sec_vendor_docs_link ?? ""} onChange={(e) => setD({ sec_vendor_docs_link: e.target.value })} placeholder="Paste link(s), separated by commas" />
                  </Field>
                </div>
              )}
            </div>
          )}
        </>
      )}

      {cat === "NON-IT" && (
        <>
          <Field label="Details of goods / services required" required>
            <textarea value={d.nit_description ?? ""} onChange={(e) => setD({ nit_description: e.target.value })} placeholder="Describe what's needed, including quantities — e.g. 40 ergonomic office chairs for the Colombo office" />
          </Field>
          <Field label="Additional specifications or document links">
            <textarea value={d.nit_specs ?? ""} onChange={(e) => setD({ nit_specs: e.target.value })} placeholder="Any additional specifications, or links to spec sheets / vendor quotes, if available" />
          </Field>
          <Field label="Business justification" required>
            <textarea value={d.business_justification ?? ""} onChange={(e) => setD({ business_justification: e.target.value })} placeholder="Why is this purchase needed? What problem does it solve, and what happens if we don't buy it?" />
          </Field>
        </>
      )}

      {cat === "EVENTS" && (
        <DevNote>
          <strong>This procurement category is under development.</strong> The Marketing &amp; Events requirement questions are still being finalized. Please continue for now — the WSO2 Procurement team will follow up with you directly for any additional details needed.
        </DevNote>
      )}
    </>
  );
}

// --- step 4 ---

function StepVendorBudget({
  cat,
  value,
  d,
  setTop,
  setD,
  attachments,
  onAttachmentsChange,
}: DetailProps & {
  value: PurchaseRequestInput;
  attachments?: File[];
  onAttachmentsChange?: (files: File[]) => void;
}) {
  const { data: vendorOptions } = useVendorLookup();
  const { data: businessUnits } = useBusinessUnitLookup();
  const { data: buApprovers } = useBusinessUnitApprovers(value.business_unit_id);

  // Selecting a business unit resets the budget approver (its approver list
  // changes). Picking an approver fills in the free-text name/email fields that
  // the backend matches on.
  const selectBusinessUnit = (id: number | null) =>
    setTop({ business_unit_id: id, budget_approver_name: "", budget_approver_email: "" });
  const selectBudgetApprover = (email: string) => {
    const u = (buApprovers ?? []).find((a) => a.email === email);
    setTop({
      budget_approver_email: email,
      budget_approver_name: u ? u.name || "" : "",
    });
  };

  const selectVendor = (v: VendorLookup) =>
    setD({
      supplier_name: v.name,
      supplier_vendor_id: v.id,
      supplier_website: v.website,
      supplier_contact: v.contact_name,
      supplier_email: v.email,
    });

  const typeSupplierName = (name: string) => {
    const match = vendorOptions?.find((v) => v.name.toLowerCase() === name.trim().toLowerCase());
    if (match) selectVendor(match);
    else setD({ supplier_name: name, supplier_vendor_id: null });
  };

  if (cat === "EVENTS") {
    return (
      <>
        <div className="pane-title">Vendor &amp; budget</div>
        <p className="pane-sub">Tell us about the proposed supplier, then complete the budget details for this purchase.</p>
        <DevNote>
          <strong>This procurement category is under development.</strong> Vendor and budget questions for Marketing &amp; Events requests are still being finalized. Please continue — the WSO2 Procurement team will coordinate vendor and budget details with you directly.
        </DevNote>
      </>
    );
  }

  return (
    <>
      <div className="pane-title">Vendor &amp; budget</div>
      <p className="pane-sub">Tell us about the proposed supplier, then complete the budget details for this purchase.</p>

      <SectionBand>Proposed vendor</SectionBand>
      <div className="grid-2">
        <Field label="Proposed supplier name (if any)" hint="Pick an existing vendor to auto-fill its details, or type a new supplier.">
          <SupplierNameCombobox value={d.supplier_name ?? ""} options={vendorOptions ?? []} onType={typeSupplierName} onSelect={selectVendor} />
          {d.supplier_vendor_id != null && <div className="help" style={{ color: "#0e9f6e" }}>✓ Linked to an existing vendor</div>}
        </Field>
        <Field label="Proposed supplier's website">
          <input type="text" value={d.supplier_website ?? ""} onChange={(e) => setD({ supplier_website: e.target.value })} placeholder="e.g. https://www.supplier.com" />
        </Field>
      </div>
      <div className={cat === "NON-IT" ? "grid-3" : "grid-2"}>
        <Field label="Contact name">
          <input type="text" value={d.supplier_contact ?? ""} onChange={(e) => setD({ supplier_contact: e.target.value })} placeholder="e.g. Jane Smith" />
        </Field>
        <Field label="Contact email">
          <input type="email" value={d.supplier_email ?? ""} onChange={(e) => setD({ supplier_email: e.target.value })} placeholder="contact@supplier.com" />
        </Field>
        {cat === "NON-IT" && (
          <Field label="Contact number">
            <input type="text" value={d.supplier_phone ?? ""} onChange={(e) => setD({ supplier_phone: e.target.value })} placeholder="e.g. +94 77 123 4567" />
          </Field>
        )}
      </div>
      {onAttachmentsChange && (
        <Field label="Attach proposal, quotation, or supporting documents" hint="Optional — PDF, DOCX, PPTX, or similar.">
          <input type="file" multiple onChange={(e) => onAttachmentsChange(Array.from(e.target.files ?? []))} />
          {attachments && attachments.length > 0 && (
            <div className="help">
              {attachments.length} file{attachments.length > 1 ? "s" : ""} selected: {attachments.map((f) => f.name).join(", ")}
            </div>
          )}
        </Field>
      )}
      <div className="info-note">
        <span className="icon">🤝</span>
        <span>Once submitted, the WSO2 Procurement team will contact this supplier directly and assess whether to proceed, in line with the Global Supply Chain Management Policy.</span>
      </div>

      <SectionBand>Budget details</SectionBand>
      <div className="grid-2">
        <Field label="Budget category">
          <input type="text" value={d.budget_category ?? ""} onChange={(e) => setD({ budget_category: e.target.value })} placeholder="e.g. Software & SaaS, Hardware & Equipment" />
        </Field>
        <Field label="Product">
          <input type="text" value={d.budget_product ?? ""} onChange={(e) => setD({ budget_product: e.target.value })} placeholder="e.g. WSO2 Identity Server, Choreo, Corporate / shared" />
        </Field>
        <Field label="Region">
          <input type="text" value={d.budget_region ?? ""} onChange={(e) => setD({ budget_region: e.target.value })} placeholder="e.g. APAC — Sri Lanka, EMEA — UK, Global" />
        </Field>
      </div>
      <Field label="Engagement code" hint="Finance / NetSuite cost-center code, if known.">
        <input type="text" value={d.engagement_code ?? ""} onChange={(e) => setD({ engagement_code: e.target.value })} placeholder="e.g. ENG-1001, if known" />
      </Field>

      <SectionBand>Budget approval</SectionBand>
      <div className="grid-2">
        <Field label="Business unit" required hint="The business unit whose budget funds this purchase.">
          <select
            value={value.business_unit_id ?? ""}
            onChange={(e) => selectBusinessUnit(e.target.value === "" ? null : Number(e.target.value))}
          >
            <option value="">— Select business unit —</option>
            {(businessUnits ?? []).map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Budget approver" required hint="Chosen from the selected business unit's approvers.">
          <select
            value={value.budget_approver_email ?? ""}
            onChange={(e) => selectBudgetApprover(e.target.value)}
            disabled={!value.business_unit_id}
          >
            <option value="">
              {!value.business_unit_id ? "Select a business unit first" : "— Select approver —"}
            </option>
            {(buApprovers ?? []).map((u) => (
              <option key={u.id} value={u.email}>
                {u.name ? `${u.name} (${u.email})` : u.email}
              </option>
            ))}
          </select>
        </Field>
      </div>

      <SectionBand>Notes for the Procurement team</SectionBand>
      <Field label="">
        <textarea value={d.notes ?? ""} onChange={(e) => setD({ notes: e.target.value })} placeholder="Anything else Procurement should know — deadlines, existing quotes, contract references…" />
      </Field>
    </>
  );
}

// --- step 5 ---

function ReviewBlock({ title, stepN, onStep, rows }: { title: string; stepN: number; onStep: (n: number) => void; rows: [string, string][] }) {
  return (
    <div className="review-block">
      <header>
        <h4>{title}</h4>
        <button type="button" onClick={() => onStep(stepN)}>
          Edit
        </button>
      </header>
      <div className="review-rows">
        {rows.map(([k, v], i) => (
          <div className="review-row" key={i}>
            <span className="k">{k}</span>
            <span className="v">{v}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function StepReview({
  value,
  d,
  onStep,
  requireDeclaration,
  declared,
  setDeclared,
}: {
  value: PurchaseRequestInput;
  d: PRDetails;
  onStep: (n: number) => void;
  requireDeclaration: boolean;
  declared: boolean;
  setDeclared: (b: boolean) => void;
}) {
  const cat = value.category;
  const { data: businessUnits } = useBusinessUnitLookup();
  const businessUnitName =
    (businessUnits ?? []).find((b) => b.id === value.business_unit_id)?.name ?? "";
  const nonEmpty = (arr?: string[]) => (arr ?? []).map((s) => s.trim()).filter(Boolean);
  const yn = (v?: YesNo) => (v ? v.toUpperCase() : "");

  const requirementRows: [string, string][] = [];
  if (cat === "IT") {
    requirementRows.push(
      ["Product / solution name", d.it_product ?? ""],
      ["Plan / subscription tier", d.it_plan ?? ""],
      ["Description", d.it_description ?? ""],
      ["Users", [d.it_user_count, nonEmpty(d.it_user_names).join(", ")].filter(Boolean).join(" — ")],
      ["Administrators", [d.it_admin_count, nonEmpty(d.it_admin_names).join(", ")].filter(Boolean).join(" — ")],
      ["Expected usage period", d.it_usage ?? ""],
      ["Business justification", d.business_justification ?? ""],
      ["Sensitive / confidential data", yn(d.sec_sensitive)],
      ["External PII", yn(d.sec_external_pii) + (d.sec_external_pii === "yes" ? ` — ${d.sec_external_pii_detail ?? ""}` : "")],
      ["Internal employee PII", yn(d.sec_employee_pii) + (d.sec_employee_pii === "yes" ? ` — ${d.sec_employee_pii_detail ?? ""}` : "")],
      [
        "WSO2 system integrations",
        d.sec_integrates === "yes"
          ? `YES — ${d.sec_integration_systems ?? ""} (${d.sec_integration_kind ?? ""}); vendor docs: ${yn(d.sec_vendor_docs)}${d.sec_vendor_docs === "yes" ? ` — ${d.sec_vendor_docs_link ?? ""}` : ""}`
          : yn(d.sec_integrates),
      ],
    );
  } else if (cat === "NON-IT") {
    requirementRows.push(
      ["Goods / services required", d.nit_description ?? ""],
      ["Additional specifications / links", d.nit_specs ?? ""],
      ["Business justification", d.business_justification ?? ""],
    );
  } else if (cat === "EVENTS") {
    requirementRows.push(["Status", "Marketing & Events requirement form is under development — the Procurement team will follow up directly."]);
  }

  const budgetRows: [string, string][] = [];
  if (cat === "IT" || cat === "NON-IT") {
    budgetRows.push(
      ["Proposed supplier", [d.supplier_name, d.supplier_website].filter(Boolean).join(" — ")],
      ["Supplier contact", [d.supplier_contact, d.supplier_email, d.supplier_phone].filter(Boolean).join(" — ")],
      ["Budget coding", [d.budget_category, d.budget_product, d.budget_region, d.engagement_code].filter(Boolean).join(" · ")],
      ["Business unit", businessUnitName],
      ["Budget approver", [value.budget_approver_name, value.budget_approver_email].filter(Boolean).join(" — ")],
      ["Notes to Procurement", d.notes ?? ""],
    );
  } else if (cat === "EVENTS") {
    budgetRows.push(["Status", "Vendor and budget details for Marketing & Events are under development — Procurement will coordinate directly."]);
  }

  return (
    <>
      <div className="pane-title">Review &amp; submit</div>
      <p className="pane-sub">Please confirm everything below is correct. On submission your requisition receives a PR reference and is routed for approval.</p>

      <ReviewBlock
        title="1 · Requester details"
        stepN={1}
        onStep={onStep}
        rows={[
          ["Date", d.date ?? ""],
          ["Full name", d.requester_name ?? ""],
          ["WSO2 email", d.requester_email ?? ""],
          ["Team lead", value.team_lead_email ?? ""],
        ]}
      />
      <ReviewBlock title="2 · Procurement Category" stepN={2} onStep={onStep} rows={[["Procurement category", cat ? CATEGORY_LABEL[cat] : ""]]} />
      <ReviewBlock title="3 · Requirement Details" stepN={3} onStep={onStep} rows={requirementRows} />
      <ReviewBlock title="4 · Vendor & budget" stepN={4} onStep={onStep} rows={budgetRows} />

      {requireDeclaration && (
        <label className="declaration">
          <input type="checkbox" checked={declared} onChange={(e) => setDeclared(e.target.checked)} />
          <span>
            I confirm the information is accurate and complete, and authorise the Procurement Team to proceed. No commitment may be made to a vendor until a formal Purchase Order is issued.
          </span>
        </label>
      )}
    </>
  );
}

// emptyRequisition builds a blank input, optionally prefilled with the requester.
export function emptyRequisition(name = "", email = ""): PurchaseRequestInput {
  const today = new Date().toISOString().slice(0, 10);
  return {
    title: "",
    business_unit_id: null,
    comments: "",
    items: [],
    links: [],
    team: "",
    entity: "",
    category: "",
    estimated_value: 0,
    currency: "USD",
    budget_approver_name: "",
    budget_approver_email: "",
    team_lead_email: "",
    details: { date: today, requester_name: name, requester_email: email },
  };
}

// requisitionTitle derives a list/summary title from the form contents.
export function requisitionTitle(value: PurchaseRequestInput): string {
  const d = value.details ?? {};
  if (value.category === "IT") return d.it_product?.trim() || "IT requisition";
  if (value.category === "NON-IT") return d.nit_description?.trim()?.slice(0, 80) || "Non-IT requisition";
  if (value.category === "EVENTS") return "Marketing & Events requisition";
  return "Purchase requisition";
}
