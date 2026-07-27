import { useMyRequests } from "../hooks/usePurchaseRequests";
import { useMe } from "../hooks/useMe";
import { PurchaseRequestsList } from "../components/PurchaseRequestsList";

// MyRequestsPage lists only the caller's own submissions ("My requests" tab),
// visible to every user regardless of role.
export function MyRequestsPage() {
  const { data, isLoading, error } = useMyRequests();
  const { data: me } = useMe();

  return (
    <PurchaseRequestsList
      title="My requests"
      subtitle="Track and manage the purchasing requests you've submitted."
      data={data}
      isLoading={isLoading}
      error={error}
      me={me}
      emptyMessage="You haven't submitted any requests yet"
      canCreate
    />
  );
}
