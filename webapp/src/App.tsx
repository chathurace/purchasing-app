import { Navigate, Route, Routes } from "react-router-dom";
import { RequireAuth } from "./auth/RequireAuth";
import { Layout } from "./components/Layout";
import { LoginPage } from "./pages/LoginPage";
import { CallbackPage } from "./pages/CallbackPage";
import { HomePage } from "./pages/HomePage";
import { PurchaseRequestListPage } from "./pages/PurchaseRequestListPage";
import { MyRequestsPage } from "./pages/MyRequestsPage";
import { NewPurchaseRequestPage } from "./pages/NewPurchaseRequestPage";
import { PurchaseRequestDetailPage } from "./pages/PurchaseRequestDetailPage";
import { ApprovalsListPage } from "./pages/ApprovalsListPage";
import { QuotationListPage } from "./pages/QuotationListPage";
import { QuotationDetailPage } from "./pages/QuotationDetailPage";
import { ContractListPage } from "./pages/ContractListPage";
import { ContractDetailPage } from "./pages/ContractDetailPage";
import { NewGrnPage } from "./pages/NewGrnPage";
import { GrnDetailPage } from "./pages/GrnDetailPage";
import { GRNListPage } from "./pages/GRNListPage";
import { NewInvoicePage } from "./pages/NewInvoicePage";
import { InvoiceDetailPage } from "./pages/InvoiceDetailPage";
import { InvoiceListPage } from "./pages/InvoiceListPage";
import { UserManagementPage } from "./pages/UserManagementPage";
import { AuditEventsPage } from "./pages/AuditEventsPage";
import { VendorListPage } from "./pages/VendorListPage";
import { VendorDetailPage } from "./pages/VendorDetailPage";
import { BusinessUnitListPage } from "./pages/BusinessUnitListPage";
import { BusinessUnitDetailPage } from "./pages/BusinessUnitDetailPage";
import { SettingsPage } from "./pages/SettingsPage";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/callback" element={<CallbackPage />} />
      <Route
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        {/* Role-based home/dashboard — the post-login landing page. */}
        <Route path="/" element={<HomePage />} />
        <Route path="/my-requests" element={<MyRequestsPage />} />
        <Route path="/requests" element={<PurchaseRequestListPage />} />
        <Route path="/requests/new" element={<NewPurchaseRequestPage />} />
        <Route path="/requests/:id" element={<PurchaseRequestDetailPage />} />

        <Route path="/approvals" element={<ApprovalsListPage />} />

        <Route path="/quotations" element={<QuotationListPage />} />
        <Route path="/quotations/:id" element={<QuotationDetailPage />} />

        <Route path="/contracts" element={<ContractListPage />} />
        <Route path="/contracts/:id" element={<ContractDetailPage />} />
        <Route path="/contracts/:id/grns/new" element={<NewGrnPage />} />
        <Route path="/contracts/:id/invoices/new" element={<NewInvoicePage />} />

        <Route path="/grns" element={<GRNListPage />} />
        <Route path="/grns/:id" element={<GrnDetailPage />} />

        <Route path="/invoices" element={<InvoiceListPage />} />
        <Route path="/invoices/:id" element={<InvoiceDetailPage />} />

        <Route path="/vendors" element={<VendorListPage />} />
        <Route path="/vendors/:id" element={<VendorDetailPage />} />

        <Route path="/business-units" element={<BusinessUnitListPage />} />
        <Route path="/business-units/:id" element={<BusinessUnitDetailPage />} />

        <Route path="/users" element={<UserManagementPage />} />
        <Route path="/audit" element={<AuditEventsPage />} />
        <Route path="/settings" element={<SettingsPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
