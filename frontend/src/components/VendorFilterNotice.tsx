import { Link } from "react-router-dom";
import { vendorRef } from "../types/api";

// Shown on the entity list pages when they are filtered to a single vendor via
// the `?vendor=<id>` query param (e.g. from a vendor's "where used" panel).
export function VendorFilterNotice({
  vendorId,
  vendorName,
  basePath,
}: {
  vendorId: number;
  vendorName?: string;
  basePath: string;
}) {
  return (
    <div className="mb-4 flex items-center justify-between rounded border border-indigo-200 bg-indigo-50 px-4 py-2 text-sm">
      <span className="text-indigo-800">
        Showing records for{" "}
        <span className="font-medium">{vendorName || vendorRef(vendorId)}</span>
      </span>
      <Link to={basePath} className="text-indigo-600 hover:underline">
        Show all
      </Link>
    </div>
  );
}
