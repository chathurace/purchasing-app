import { useState } from "react";
import { useConfigLookup, optionsFor } from "../hooks/useConfigOptions";
import { useCostCenterLookup } from "../hooks/useCostCenters";
import { useVendorLookup } from "../hooks/useVendors";
import { SupplierNameCombobox } from "./SupplierNameCombobox";
import type {
  CostCenterSummary,
  PRCategory,
  PRDetails,
  PurchaseRequestInput,
  VendorLookup,
  YesNo,
  YesNoUnknown,
} from "../types/api";

const inputCls = "w-full rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none";
const labelCls = "mb-1 block text-xs font-semibold uppercase tracking-wide text-gray-500";
const subCls =
  "mb-4 rounded-r border-l-4 border-indigo-500 bg-gray-50 px-3 py-2 text-xs font-semibold uppercase tracking-wide text-gray-600";

const STEPS = ["Requester", "Purchase", "Vendor & budget", "Review"];

interface Props {
  value: PurchaseRequestInput;
  onChange: (next: PurchaseRequestInput) => void;
  onSubmit: () => void;
  submitting: boolean;
  submitLabel: string;
  requireDeclaration?: boolean;
  error?: string | null;
}

export function RequisitionForm({
  value,
  onChange,
  onSubmit,
  submitting,
  submitLabel,
  requireDeclaration = false,
  error,
}: Props) {
  const [step, setStep] = useState(1);
  const [declared, setDeclared] = useState(false);
  const [vErr, setVErr] = useState<string | null>(null);

  const { data: config } = useConfigLookup();
  const lists = config?.lists;
  const { data: costCenters } = useCostCenterLookup();

  const d: PRDetails = value.details ?? {};
  const setTop = (patch: Partial<PurchaseRequestInput>) => onChange({ ...value, ...patch });
  const setD = (patch: Partial<PRDetails>) => onChange({ ...value, details: { ...d, ...patch } });

  const goTo = (n: number) => {
    setVErr(null);
    setStep(n);
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  // Returns the labels of required fields missing on a given step.
  const missingOn = (n: number): string[] => {
    const need: [boolean, string][] = [];
    if (n === 1) {
      need.push(
        [!d.date, "Date"],
        [!d.requester_name?.trim(), "Your full name"],
        [!d.requester_email?.trim(), "WSO2 email"],
        [!value.cost_center_id, "Cost center"],
        [!value.entity, "WSO2 entity"],
        [!d.business_justification?.trim(), "Business justification"],
      );
    } else if (n === 2) {
      if (!value.category) need.push([true, "Purchase type"]);
      else if (value.category === "IT") {
        need.push(
          [!d.it_category, "IT category"],
          [!d.it_product?.trim(), "Product / solution name"],
          [!d.it_description?.trim(), "Description"],
          [!d.it_plan?.trim(), "Plan / tier"],
          [!d.it_admins?.trim(), "Administrators"],
          [!d.it_usage?.trim(), "Day-to-day usage"],
          [!d.sec_sensitive, "Sensitive data answer"],
          [!d.sec_external_pii, "External PII answer"],
          [!d.sec_employee_pii, "Employee PII answer"],
          [!d.sec_integrates, "Integration answer"],
        );
      } else {
        need.push([!d.nit_category, "Non-IT category"], [!d.nit_description?.trim(), "Details of goods / services"]);
      }
    } else if (n === 3) {
      // Proposed supplier & commercial details are optional sections; only the
      // budget approval and budget-coding fields are required.
      need.push(
        [!value.budget_approver_name?.trim(), "Budget approver name"],
        [!value.budget_approver_email?.trim(), "Budget approver email"],
        [!d.budget_category, "Budget category"],
        [!d.budget_cost_center, "Cost center"],
        [!d.budget_product, "Product"],
        [!d.budget_region, "Region"],
      );
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
    for (let n = 1; n <= 3; n++) {
      const missing = missingOn(n);
      if (missing.length) {
        setStep(n);
        setVErr("Please complete: " + missing.join(", "));
        return;
      }
    }
    if (requireDeclaration && !declared) {
      setStep(4);
      setVErr("Please confirm the declaration before submitting.");
      return;
    }
    setVErr(null);
    onSubmit();
  };

  return (
    <div className="rounded border bg-white">
      <Stepper step={step} onStep={goTo} />

      <div className="p-6">
        {step === 1 && <StepRequester value={value} d={d} setTop={setTop} setD={setD} lists={lists} costCenters={costCenters} />}
        {step === 2 && <StepPurchase value={value} d={d} setTop={setTop} setD={setD} lists={lists} />}
        {step === 3 && <StepVendor value={value} d={d} setTop={setTop} setD={setD} lists={lists} costCenters={costCenters} />}
        {step === 4 && (
          <StepReview
            value={value}
            d={d}
            requireDeclaration={requireDeclaration}
            declared={declared}
            setDeclared={setDeclared}
          />
        )}

        {(vErr || error) && <p className="mt-4 text-sm text-red-600">{vErr || error}</p>}

        <div className="mt-6 flex justify-between border-t pt-4">
          <button
            type="button"
            onClick={() => goTo(step - 1)}
            disabled={step === 1}
            className="rounded border px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-40"
          >
            Back
          </button>
          {step < 4 ? (
            <button
              type="button"
              onClick={next}
              className="rounded bg-indigo-600 px-5 py-2 text-sm font-medium text-white hover:bg-indigo-700"
            >
              {step === 3 ? "Review" : "Next"}
            </button>
          ) : (
            <button
              type="button"
              onClick={submit}
              disabled={submitting}
              className="rounded bg-indigo-600 px-5 py-2 text-sm font-semibold text-white hover:bg-indigo-700 disabled:opacity-50"
            >
              {submitting ? "Submitting…" : submitLabel}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

// --- stepper header ---

function Stepper({ step, onStep }: { step: number; onStep: (n: number) => void }) {
  return (
    <div>
      <div className="flex border-b">
        {STEPS.map((label, i) => {
          const n = i + 1;
          const active = n === step;
          const done = n < step;
          return (
            <button
              type="button"
              key={label}
              onClick={() => onStep(n)}
              className={`flex flex-1 items-center justify-center gap-2 border-r px-2 py-3 text-xs font-medium last:border-r-0 ${
                active ? "bg-white text-gray-900" : "bg-gray-50 text-gray-500 hover:text-gray-700"
              }`}
            >
              <span
                className={`flex h-5 w-5 items-center justify-center rounded-full text-[11px] font-semibold ${
                  active
                    ? "bg-indigo-600 text-white"
                    : done
                      ? "bg-green-100 text-green-700"
                      : "bg-gray-200 text-gray-500"
                }`}
              >
                {done ? "✓" : n}
              </span>
              <span className="hidden sm:inline">{label}</span>
            </button>
          );
        })}
      </div>
      <div className="h-0.5 bg-gray-200">
        <div className="h-full bg-indigo-600 transition-all" style={{ width: `${(step / 4) * 100}%` }} />
      </div>
    </div>
  );
}

// --- shared field helpers ---

function Field({ label, required, hint, children }: { label: string; required?: boolean; hint?: string; children: React.ReactNode }) {
  return (
    <div>
      <label className={labelCls}>
        {label}
        {required && <span className="ml-0.5 text-indigo-600">*</span>}
      </label>
      {children}
      {hint && <p className="mt-1 text-xs italic text-gray-400">{hint}</p>}
    </div>
  );
}

function Select({ value, onChange, options, placeholder }: { value?: string; onChange: (v: string) => void; options: string[]; placeholder: string }) {
  return (
    <select className={inputCls} value={value ?? ""} onChange={(e) => onChange(e.target.value)}>
      <option value="">{placeholder}</option>
      {options.map((o) => (
        <option key={o} value={o}>
          {o}
        </option>
      ))}
    </select>
  );
}

function YesNoField({ value, onChange }: { value?: YesNo; onChange: (v: YesNo) => void }) {
  const base = "flex-1 rounded border px-3 py-2 text-sm font-medium transition";
  return (
    <div className="flex gap-2">
      <button
        type="button"
        onClick={() => onChange("yes")}
        className={`${base} ${value === "yes" ? "border-green-600 bg-green-50 text-green-700" : "border-gray-200 text-gray-500 hover:border-indigo-400"}`}
      >
        Yes
      </button>
      <button
        type="button"
        onClick={() => onChange("no")}
        className={`${base} ${value === "no" ? "border-red-500 bg-red-50 text-red-600" : "border-gray-200 text-gray-500 hover:border-indigo-400"}`}
      >
        No
      </button>
    </div>
  );
}

// Three-way variant of YesNoField, used for "within approved budget?" where the
// requester may genuinely not know yet.
function YesNoUnknownField({ value, onChange }: { value?: YesNoUnknown; onChange: (v: YesNoUnknown) => void }) {
  const base = "flex-1 rounded border px-3 py-2 text-sm font-medium transition";
  const opts: [YesNoUnknown, string, string][] = [
    ["yes", "Yes", "border-green-600 bg-green-50 text-green-700"],
    ["no", "No", "border-red-500 bg-red-50 text-red-600"],
    ["unknown", "Don't know", "border-amber-500 bg-amber-50 text-amber-700"],
  ];
  return (
    <div className="flex gap-2">
      {opts.map(([v, label, on]) => (
        <button
          key={v}
          type="button"
          onClick={() => onChange(v)}
          className={`${base} ${value === v ? on : "border-gray-200 text-gray-500 hover:border-indigo-400"}`}
        >
          {label}
        </button>
      ))}
    </div>
  );
}

// Collapsible groups an optional section behind a <details> disclosure, collapsed
// by default. The chevron rotates open via the group-open utility.
function Collapsible({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <details className="group rounded border border-gray-200">
      <summary className="flex cursor-pointer list-none items-center justify-between px-3 py-2 text-xs font-semibold uppercase tracking-wide text-gray-600">
        <span>
          {title} <span className="ml-1 normal-case tracking-normal text-gray-400">(optional)</span>
        </span>
        <span className="text-gray-400 transition group-open:rotate-90">▶</span>
      </summary>
      <div className="space-y-4 border-t border-gray-100 p-3">{children}</div>
    </details>
  );
}

interface StepProps {
  value: PurchaseRequestInput;
  d: PRDetails;
  setTop: (patch: Partial<PurchaseRequestInput>) => void;
  setD: (patch: Partial<PRDetails>) => void;
  lists?: Record<string, string[]>;
  costCenters?: CostCenterSummary[];
}

// CostCenterSelect binds the requisition's cost_center_id (and a display name in
// cost_center) to a managed cost center. If the PR already references a cost
// center that is no longer in the active lookup, it is still shown so editing an
// existing PR never silently drops the selection.
function CostCenterSelect({
  value,
  currentLabel,
  costCenters,
  onChange,
}: {
  value: number | null;
  currentLabel?: string;
  costCenters?: CostCenterSummary[];
  onChange: (cc: CostCenterSummary | null, label: string) => void;
}) {
  const options = costCenters ?? [];
  const label = (c: CostCenterSummary) => (c.code ? `${c.code} — ${c.name}` : c.name);
  const missingCurrent = value != null && !options.some((c) => c.id === value);
  return (
    <select
      className={inputCls}
      value={value ?? ""}
      onChange={(e) => {
        const id = e.target.value ? Number(e.target.value) : null;
        const cc = options.find((c) => c.id === id) ?? null;
        onChange(cc, cc ? label(cc) : "");
      }}
    >
      <option value="">Select cost center</option>
      {missingCurrent && (
        <option value={value}>{currentLabel?.trim() ? currentLabel : `Cost center #${value}`}</option>
      )}
      {options.map((c) => (
        <option key={c.id} value={c.id}>
          {label(c)}
        </option>
      ))}
    </select>
  );
}

// --- step 1 ---

function StepRequester({ value, d, setTop, setD, lists, costCenters }: StepProps) {
  return (
    <div className="space-y-4">
      <SectionTitle title="Requester details" desc="Tell us who you are and why this purchase is needed." />
      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="Date" required>
          <input type="date" className={inputCls} value={d.date ?? ""} onChange={(e) => setD({ date: e.target.value })} />
        </Field>
        <Field label="Your full name" required>
          <input className={inputCls} value={d.requester_name ?? ""} onChange={(e) => setD({ requester_name: e.target.value })} placeholder="First and last name" />
        </Field>
        <Field label="WSO2 email" required>
          <input type="email" className={inputCls} value={d.requester_email ?? ""} onChange={(e) => setD({ requester_email: e.target.value })} placeholder="you@wso2.com" />
        </Field>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Cost center" required hint="Determines who owns the budget approval — the approver is filled in from its owner.">
          <CostCenterSelect
            value={value.cost_center_id}
            currentLabel={value.cost_center}
            costCenters={costCenters}
            onChange={(cc, name) =>
              // Single update so the top-level and details patches don't race.
              setTop({
                cost_center_id: cc?.id ?? null,
                cost_center: name,
                // Autofill the budget approver from the cost-center owner; the
                // requester can still override it in step 3.
                budget_approver_name: cc?.owner_name ?? "",
                budget_approver_email: cc?.owner_email ?? "",
                // Default the budget-coding cost center to the same pick, but
                // never overwrite one the requester already set there.
                details: d.budget_cost_center_id
                  ? d
                  : { ...d, budget_cost_center: name, budget_cost_center_id: cc?.id ?? null },
              })
            }
          />
        </Field>
        <Field label="WSO2 entity" required>
          <Select value={value.entity} onChange={(entity) => setTop({ entity })} options={optionsFor(lists, "entity", value.entity)} placeholder="Select entity" />
        </Field>
      </div>
      <Field label="Business justification" required>
        <textarea className={`${inputCls} min-h-[90px]`} value={d.business_justification ?? ""} onChange={(e) => setD({ business_justification: e.target.value })} placeholder="Why is this purchase needed? What problem does it solve?" />
      </Field>
    </div>
  );
}

// --- step 2 ---

function StepPurchase({ value, d, setTop, setD, lists }: StepProps) {
  const cat = value.category;
  const catBtn = (c: PRCategory, title: string, sub: string) => (
    <button
      type="button"
      onClick={() => setTop({ category: c })}
      className={`flex-1 rounded border px-4 py-3 text-left transition ${
        cat === c ? "border-indigo-600 bg-indigo-50" : "border-gray-200 hover:border-indigo-400"
      }`}
    >
      <div className="text-sm font-semibold text-gray-900">{title}</div>
      <div className="text-xs text-gray-500">{sub}</div>
    </button>
  );

  return (
    <div className="space-y-4">
      <SectionTitle title="Purchase details" desc="Pick the type of purchase and complete the relevant section." />
      <div className="flex gap-3">
        {catBtn("IT", "IT solution", "Software, SaaS, cloud, hardware")}
        {catBtn("NON-IT", "Non-IT solution", "Facilities, insurance, goods, services")}
      </div>

      {cat === "IT" && (
        <div className="space-y-4">
          <div className={subCls}>Section A — IT solution details</div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="IT category" required>
              <Select value={d.it_category} onChange={(v) => setD({ it_category: v })} options={optionsFor(lists, "it_category", d.it_category)} placeholder="Select category" />
            </Field>
            <Field label="Product / solution name" required>
              <input className={inputCls} value={d.it_product ?? ""} onChange={(e) => setD({ it_product: e.target.value })} placeholder="e.g. Salesforce, GitHub Enterprise" />
            </Field>
          </div>
          <Field label="Description of the solution" required>
            <textarea className={`${inputCls} min-h-[80px]`} value={d.it_description ?? ""} onChange={(e) => setD({ it_description: e.target.value })} placeholder="What does it do and how will WSO2 use it?" />
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Plan / subscription tier" required>
              <input className={inputCls} value={d.it_plan ?? ""} onChange={(e) => setD({ it_plan: e.target.value })} placeholder="e.g. Enterprise, Pro" />
            </Field>
            <Field label="Expected number of users">
              <input className={inputCls} value={d.it_users ?? ""} onChange={(e) => setD({ it_users: e.target.value })} placeholder="e.g. 20 — Sales team" />
            </Field>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Administrators (names & count)" required>
              <input className={inputCls} value={d.it_admins ?? ""} onChange={(e) => setD({ it_admins: e.target.value })} placeholder="e.g. 2 — John S., Maria L." />
            </Field>
            <Field label="Expected day-to-day usage" required>
              <input className={inputCls} value={d.it_usage ?? ""} onChange={(e) => setD({ it_usage: e.target.value })} placeholder="How will this be used?" />
            </Field>
          </div>

          <div className={subCls}>Data &amp; security assessment</div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Stores / processes sensitive WSO2 data?" required>
              <YesNoField value={d.sec_sensitive} onChange={(v) => setD({ sec_sensitive: v })} />
            </Field>
            <Field label="Captures external PII (customers / prospects)?" required>
              <YesNoField value={d.sec_external_pii} onChange={(v) => setD({ sec_external_pii: v })} />
              {d.sec_external_pii === "yes" && (
                <input className={`${inputCls} mt-2`} value={d.sec_external_pii_detail ?? ""} onChange={(e) => setD({ sec_external_pii_detail: e.target.value })} placeholder="Specify what PII will be captured" />
              )}
            </Field>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Captures internal employee PII?" required>
              <YesNoField value={d.sec_employee_pii} onChange={(v) => setD({ sec_employee_pii: v })} />
            </Field>
            <Field label="Integrates with WSO2 internal systems?" required>
              <YesNoField value={d.sec_integrates} onChange={(v) => setD({ sec_integrates: v })} />
              {d.sec_integrates === "yes" && (
                <textarea className={`${inputCls} mt-2 min-h-[60px]`} value={d.sec_integration_detail ?? ""} onChange={(e) => setD({ sec_integration_detail: e.target.value })} placeholder="Which systems? What kind of integration (API, SSO, data sync)?" />
              )}
            </Field>
          </div>
        </div>
      )}

      {cat === "NON-IT" && (
        <div className="space-y-4">
          <div className={subCls}>Section B — Non-IT solution details</div>
          <Field label="Non-IT category" required>
            <Select value={d.nit_category} onChange={(v) => setD({ nit_category: v })} options={optionsFor(lists, "nonit_category", d.nit_category)} placeholder="Select category" />
          </Field>
          <Field label="Details of goods / services required" required>
            <textarea className={`${inputCls} min-h-[90px]`} value={d.nit_description ?? ""} onChange={(e) => setD({ nit_description: e.target.value })} placeholder="Describe what you need: quantities, specifications, relevant details…" />
          </Field>
          <Field label="Additional specifications or document links">
            <textarea className={`${inputCls} min-h-[60px]`} value={d.nit_specs ?? ""} onChange={(e) => setD({ nit_specs: e.target.value })} placeholder="Any links, spec sheets, or context…" />
          </Field>
        </div>
      )}

      {!cat && <p className="text-sm text-gray-400">Select a purchase type above to continue.</p>}
    </div>
  );
}

// --- step 3 ---

function StepVendor({ value, d, setTop, setD, lists, costCenters }: StepProps) {
  const { data: vendorOptions } = useVendorLookup();

  // The supplier name is an editable combobox over the vendor master. Picking
  // (or typing the exact name of) a known vendor auto-fills the other supplier
  // fields and records the vendor id; any other text is a free-typed supplier
  // and is never added to the vendor master.
  const selectVendor = (v: VendorLookup) =>
    setD({
      supplier_name: v.name,
      supplier_vendor_id: v.id,
      supplier_website: v.website,
      supplier_contact: v.contact_name,
      supplier_email: v.email,
    });

  const typeSupplierName = (name: string) => {
    const match = vendorOptions?.find(
      (v) => v.name.toLowerCase() === name.trim().toLowerCase(),
    );
    if (match) selectVendor(match);
    else setD({ supplier_name: name, supplier_vendor_id: null });
  };

  return (
    <div className="space-y-4">
      <SectionTitle title="Vendor & budget" desc="The proposed supplier and the budget owner for this purchase." />

      <Collapsible title="Proposed supplier">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field
            label="Supplier name"
            hint="Pick an existing vendor to auto-fill its details, or type a new supplier."
          >
            <SupplierNameCombobox
              value={d.supplier_name ?? ""}
              options={vendorOptions ?? []}
              onType={typeSupplierName}
              onSelect={selectVendor}
            />
            {d.supplier_vendor_id != null && (
              <p className="mt-1 text-xs text-green-700">✓ Linked to an existing vendor</p>
            )}
          </Field>
          <Field label="Supplier website">
            <input className={inputCls} value={d.supplier_website ?? ""} onChange={(e) => setD({ supplier_website: e.target.value })} placeholder="https://…" />
          </Field>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Supplier contact person">
            <input className={inputCls} value={d.supplier_contact ?? ""} onChange={(e) => setD({ supplier_contact: e.target.value })} placeholder="Account manager / sales rep" />
          </Field>
          <Field label="Supplier contact email">
            <input type="email" className={inputCls} value={d.supplier_email ?? ""} onChange={(e) => setD({ supplier_email: e.target.value })} placeholder="contact@supplier.com" />
          </Field>
        </div>
        <Field label="Has this supplier worked with WSO2 before?" hint="New vendors must complete an RFI — the Procurement Team will guide you.">
          <YesNoField value={d.supplier_existing} onChange={(v) => setD({ supplier_existing: v })} />
        </Field>
      </Collapsible>

      <Collapsible title="Commercial details">
        <div className="grid gap-4 sm:grid-cols-3">
          <Field label="Estimated value">
            <input type="number" min={0} step="0.01" className={inputCls} value={value.estimated_value || ""} onChange={(e) => setTop({ estimated_value: Number(e.target.value) })} placeholder="0.00" />
          </Field>
          <Field label="Currency">
            <Select value={value.currency} onChange={(currency) => setTop({ currency })} options={optionsFor(lists, "currency", value.currency)} placeholder="Select" />
          </Field>
          <Field label="Engagement type">
            <Select value={d.engagement_type} onChange={(v) => setD({ engagement_type: v })} options={optionsFor(lists, "engagement_type", d.engagement_type)} placeholder="Select" />
          </Field>
        </div>
        <Field label="Is this within the approved budget?">
          <YesNoUnknownField value={d.within_budget} onChange={(v) => setD({ within_budget: v })} />
        </Field>
      </Collapsible>

      <div className={subCls}>Budget approval</div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Budget approver's name" required hint="Filled in from the cost-centre owner — edit if someone else approves.">
          <input className={inputCls} value={value.budget_approver_name ?? ""} onChange={(e) => setTop({ budget_approver_name: e.target.value })} placeholder="Name of the budget owner" />
        </Field>
        <Field label="Budget approver's email" required>
          <input type="email" className={inputCls} value={value.budget_approver_email ?? ""} onChange={(e) => setTop({ budget_approver_email: e.target.value })} placeholder="approver@wso2.com" />
        </Field>
      </div>

      <div className={subCls}>Budget coding</div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Budget category" required>
          <Select value={d.budget_category} onChange={(v) => setD({ budget_category: v })} options={optionsFor(lists, "budget_category", d.budget_category)} placeholder="Select category" />
        </Field>
        <Field label="Cost center" required>
          <CostCenterSelect
            value={d.budget_cost_center_id ?? null}
            currentLabel={d.budget_cost_center}
            costCenters={costCenters}
            onChange={(_, name) =>
              setD({ budget_cost_center: name, budget_cost_center_id: _?.id ?? null })
            }
          />
        </Field>
        <Field label="Product" required>
          <Select value={d.budget_product} onChange={(v) => setD({ budget_product: v })} options={optionsFor(lists, "product", d.budget_product)} placeholder="Select product" />
        </Field>
        <Field label="Region" required>
          <Select value={d.budget_region} onChange={(v) => setD({ budget_region: v })} options={optionsFor(lists, "region", d.budget_region)} placeholder="Select region" />
        </Field>
      </div>
      <Field label="Engagement code" hint="Finance / NetSuite cost-centre code, if known.">
        <Select value={d.engagement_code} onChange={(v) => setD({ engagement_code: v })} options={optionsFor(lists, "engagement_code", d.engagement_code)} placeholder="Select code" />
      </Field>
      <Field label="Notes for the Procurement Team">
        <textarea className={`${inputCls} min-h-[60px]`} value={d.notes ?? ""} onChange={(e) => setD({ notes: e.target.value })} placeholder="Urgency, context, vendor commitments, special considerations…" />
      </Field>
    </div>
  );
}

// --- step 4 ---

function StepReview({
  value,
  d,
  requireDeclaration,
  declared,
  setDeclared,
}: {
  value: PurchaseRequestInput;
  d: PRDetails;
  requireDeclaration: boolean;
  declared: boolean;
  setDeclared: (b: boolean) => void;
}) {
  const rows: [string, string][] = [
    ["Requester", d.requester_name || "—"],
    ["Date", d.date || "—"],
    ["Cost center", value.cost_center || "—"],
    ["Entity", value.entity || "—"],
    ["Type", value.category === "IT" ? "IT solution" : value.category === "NON-IT" ? "Non-IT solution" : "—"],
    ["Solution", value.category === "IT" ? d.it_product || "—" : d.nit_category || "—"],
    ["Supplier", d.supplier_name || "—"],
    ["Vendor status", d.supplier_existing === "yes" ? "Registered vendor" : d.supplier_existing === "no" ? "New vendor (RFI)" : "—"],
    ["Estimated value", value.estimated_value ? `${value.currency} ${value.estimated_value.toLocaleString()}` : "—"],
    ["Budget approver", value.budget_approver_name || "—"],
    ["Budget category", d.budget_category || "—"],
    ["Cost center (coding)", d.budget_cost_center || "—"],
    ["Product", d.budget_product || "—"],
    ["Region", d.budget_region || "—"],
  ];
  return (
    <div className="space-y-4">
      <SectionTitle title="Review & submit" desc="Check the details before sending to the Procurement Team." />
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        {rows.map(([l, v]) => (
          <div key={l} className="rounded bg-gray-50 px-3 py-2">
            <div className="text-[10px] font-semibold uppercase tracking-wide text-gray-500">{l}</div>
            <div className="mt-0.5 break-words text-sm text-gray-900">{v}</div>
          </div>
        ))}
      </div>
      {requireDeclaration && (
        <label className="flex items-start gap-2 rounded border bg-gray-50 p-4 text-sm text-gray-700">
          <input type="checkbox" className="mt-0.5 accent-indigo-600" checked={declared} onChange={(e) => setDeclared(e.target.checked)} />
          <span>
            I confirm the information is accurate and complete, and authorise the Procurement Team to proceed.
            No commitment may be made to a vendor until a formal Purchase Order is issued.
          </span>
        </label>
      )}
    </div>
  );
}

function SectionTitle({ title, desc }: { title: string; desc: string }) {
  return (
    <div>
      <h2 className="inline-block border-b-2 border-indigo-500 pb-1 text-base font-semibold text-gray-900">{title}</h2>
      <p className="mt-2 text-sm text-gray-500">{desc}</p>
    </div>
  );
}

// emptyRequisition builds a blank input, optionally prefilled with the requester.
export function emptyRequisition(name = "", email = ""): PurchaseRequestInput {
  const today = new Date().toISOString().slice(0, 10);
  return {
    title: "",
    cost_center: "",
    cost_center_id: null,
    comments: "",
    items: [],
    links: [],
    approver_ids: [],
    team: "",
    entity: "",
    category: "",
    estimated_value: 0,
    currency: "USD",
    budget_approver_name: "",
    budget_approver_email: "",
    details: { date: today, requester_name: name, requester_email: email },
  };
}

// requisitionTitle derives a list/summary title from the form contents.
export function requisitionTitle(value: PurchaseRequestInput): string {
  const d = value.details ?? {};
  if (value.category === "IT") return d.it_product?.trim() || d.it_category || "IT requisition";
  if (value.category === "NON-IT") return d.nit_category || "Non-IT requisition";
  return "Purchase requisition";
}
