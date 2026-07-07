import { Navigate, Route, Routes } from "react-router-dom";
import { RequireAuth } from "./auth/RequireAuth";
import { Layout } from "./components/Layout";
import { LoginPage } from "./pages/LoginPage";
import { CallbackPage } from "./pages/CallbackPage";
import { PurchaseRequestListPage } from "./pages/PurchaseRequestListPage";
import { NewPurchaseRequestPage } from "./pages/NewPurchaseRequestPage";
import { PurchaseRequestDetailPage } from "./pages/PurchaseRequestDetailPage";
import { ApprovalsListPage } from "./pages/ApprovalsListPage";
import { QuotationListPage } from "./pages/QuotationListPage";
import { QuotationDetailPage } from "./pages/QuotationDetailPage";
import { NewContractPage } from "./pages/NewContractPage";
import { ContractListPage } from "./pages/ContractListPage";
import { ContractDetailPage } from "./pages/ContractDetailPage";
import { NewGrnPage } from "./pages/NewGrnPage";
import { GrnDetailPage } from "./pages/GrnDetailPage";
import { GRNListPage } from "./pages/GRNListPage";
import { NewInvoicePage } from "./pages/NewInvoicePage";
import { InvoiceDetailPage } from "./pages/InvoiceDetailPage";
import { InvoiceListPage } from "./pages/InvoiceListPage";
import { UserManagementPage } from "./pages/UserManagementPage";
import { VendorListPage } from "./pages/VendorListPage";
import { VendorDetailPage } from "./pages/VendorDetailPage";
import { CostCenterListPage } from "./pages/CostCenterListPage";
import { CostCenterDetailPage } from "./pages/CostCenterDetailPage";
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
        <Route path="/" element={<Navigate to="/requests" replace />} />
        <Route path="/requests" element={<PurchaseRequestListPage />} />
        <Route path="/requests/new" element={<NewPurchaseRequestPage />} />
        <Route path="/requests/:id" element={<PurchaseRequestDetailPage />} />

        <Route path="/approvals" element={<ApprovalsListPage />} />

        <Route path="/quotations" element={<QuotationListPage />} />
        <Route path="/quotations/:id" element={<QuotationDetailPage />} />
        <Route path="/quotations/:id/contracts/new" element={<NewContractPage />} />

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

        <Route path="/cost-centers" element={<CostCenterListPage />} />
        <Route path="/cost-centers/:id" element={<CostCenterDetailPage />} />

        <Route path="/users" element={<UserManagementPage />} />
        <Route path="/settings" element={<SettingsPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/requests" replace />} />
    </Routes>
  );
}
