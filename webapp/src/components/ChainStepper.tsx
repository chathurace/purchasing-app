import { Fragment } from "react";
import { Link } from "react-router-dom";
import { useFinanceAccess } from "../hooks/useFinanceAccess";
import { useRelatedDocuments } from "../hooks/usePurchaseRequests";
import { buildSteps } from "../lib/caseGraph";
import type { CaseCurrent, StepView } from "../lib/caseGraph";

// ChainStepper shows where the current record sits in the procurement chain
// (Request ▸ Quotation ▸ Contract): ancestors link to the specific record on
// this record's path, the current stage is highlighted, and downstream stages
// show how many records hang off it. Finance-access only (it reads /related).
export function ChainStepper({ prId, current }: { prId: number; current: CaseCurrent }) {
  const finance = useFinanceAccess();
  const { data } = useRelatedDocuments(prId, finance);
  if (!finance || !data) return null;

  const steps = buildSteps(data, current);
  return (
    <nav className="mb-4 flex flex-wrap items-center gap-x-1 gap-y-1 rounded border bg-white px-3 py-2">
      {steps.map((step, i) => (
        <Fragment key={step.kind}>
          {i > 0 && <span className="px-0.5 text-gray-300">›</span>}
          <Step step={step} />
        </Fragment>
      ))}
    </nav>
  );
}

function Step({ step }: { step: StepView }) {
  const base = "rounded px-2 py-0.5 text-xs font-medium";
  if (step.state === "current") {
    return (
      <span className={`${base} bg-indigo-600 text-white`}>
        {step.label}
        {step.ref ? ` · ${step.ref}` : ""}
      </span>
    );
  }
  if (step.state === "ancestor") {
    if (!step.to) {
      return <span className={`${base} text-gray-400`}>{step.label} · —</span>;
    }
    return (
      <Link to={step.to} className={`${base} text-indigo-700 hover:bg-indigo-50`}>
        {step.label} · {step.ref}
      </Link>
    );
  }
  // descendant
  return (
    <span className={`${base} font-normal text-gray-400`}>
      {step.label}
      {step.count ? ` · ${step.count}` : ""}
    </span>
  );
}
