import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "./auth";
import { NotificationsProvider } from "./notifications";
import Landing from "./pages/Landing";
import Login from "./pages/Login";
import Signup from "./pages/Signup";
import OperatorConsole from "./pages/OperatorConsole";
import HolderPortal from "./pages/HolderPortal";

export default function App() {
  const { ready } = useAuth();
  if (!ready) return <div className="center muted">One moment…</div>;

  return (
    <Routes>
      <Route path="/" element={<GuestOnly><Landing /></GuestOnly>} />
      <Route path="/login" element={<GuestOnly><Login /></GuestOnly>} />
      <Route path="/signup" element={<GuestOnly><Signup /></GuestOnly>} />
      <Route path="/app" element={<AppShell />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

// GuestOnly bounces signed-in visitors to the ledger.
function GuestOnly({ children }: { children: React.ReactNode }) {
  const { session } = useAuth();
  return session ? <Navigate to="/app" replace /> : <>{children}</>;
}

function AppShell() {
  const { session } = useAuth();
  if (!session) return <Navigate to="/login" replace />;
  return (
    <NotificationsProvider>
      {session.role === "operator" ? <OperatorConsole /> : <HolderPortal />}
    </NotificationsProvider>
  );
}
