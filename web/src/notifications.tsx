import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { api, type AppNotification } from "./api";
import { useAuth } from "./auth";

interface NotificationsState {
  notifications: AppNotification[];
  unreadCount: number;
  markRead: (id: string) => void;
  markAllRead: () => void;
  // Bumped to the most recently received live notification — pages can watch
  // this to refresh their own data (e.g. the operator's pending-approvals
  // count) without each maintaining a separate SSE connection.
  lastEvent: AppNotification | null;
}

const NotificationsContext = createContext<NotificationsState>({
  notifications: [],
  unreadCount: 0,
  markRead: () => {},
  markAllRead: () => {},
  lastEvent: null,
});

export function NotificationsProvider({ children }: { children: ReactNode }) {
  const { session } = useAuth();
  const [notifications, setNotifications] = useState<AppNotification[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [lastEvent, setLastEvent] = useState<AppNotification | null>(null);
  const esRef = useRef<EventSource | null>(null);

  const refresh = useCallback(async () => {
    const { notifications, unread_count } = await api.listNotifications();
    setNotifications(notifications ?? []);
    setUnreadCount(unread_count ?? 0);
  }, []);

  useEffect(() => {
    if (!session) {
      esRef.current?.close();
      esRef.current = null;
      setNotifications([]);
      setUnreadCount(0);
      setLastEvent(null);
      return;
    }

    refresh().catch(() => {});

    let stopped = false;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;

    const connect = async () => {
      try {
        const { ticket } = await api.notificationStreamToken();
        if (stopped) return;
        const es = new EventSource(`/api/v1/notifications/stream?ticket=${encodeURIComponent(ticket)}`);
        esRef.current = es;
        es.addEventListener("notification", (event) => {
          const n: AppNotification = JSON.parse((event as MessageEvent).data);
          setNotifications((prev) => [n, ...prev].slice(0, 100));
          setUnreadCount((c) => c + 1);
          setLastEvent(n);
        });
        es.onerror = () => {
          // The connect ticket is single-use, so the browser's built-in
          // reconnect (which replays the same URL) would just fail again —
          // close it ourselves and mint a fresh ticket instead.
          es.close();
          if (!stopped) retryTimer = setTimeout(connect, 3000);
        };
      } catch {
        if (!stopped) retryTimer = setTimeout(connect, 5000);
      }
    };
    connect();

    const onFocus = () => refresh().catch(() => {});
    window.addEventListener("focus", onFocus);

    return () => {
      stopped = true;
      clearTimeout(retryTimer);
      esRef.current?.close();
      esRef.current = null;
      window.removeEventListener("focus", onFocus);
    };
  }, [session, refresh]);

  const markRead = (id: string) => {
    setNotifications((prev) => prev.map((n) => (n.id === id ? { ...n, read_at: n.read_at ?? new Date().toISOString() } : n)));
    setUnreadCount((c) => Math.max(0, c - 1));
    api.markNotificationRead(id).catch(() => {});
  };

  const markAllRead = () => {
    const now = new Date().toISOString();
    setNotifications((prev) => prev.map((n) => ({ ...n, read_at: n.read_at ?? now })));
    setUnreadCount(0);
    api.markAllNotificationsRead().catch(() => {});
  };

  return (
    <NotificationsContext.Provider value={{ notifications, unreadCount, markRead, markAllRead, lastEvent }}>
      {children}
    </NotificationsContext.Provider>
  );
}

export function useNotifications() {
  return useContext(NotificationsContext);
}
