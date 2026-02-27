import { useEffect, useState } from "react";

const API_BASE = "http://localhost:8080";

type User = {
  id: number;
  name: string;
};

type Order = {
  id: number;
  buyer_id: number;
  seller_id: number;
  amount: number;
  status: string;
};

async function jsonFetch<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    headers: {
      "Content-Type": "application/json",
    },
    ...options,
  });
  if (!res.ok) {
    let message = `HTTP ${res.status}`;
    try {
      const body = await res.json();
      if (body?.error) message = body.error;
    } catch {
      // ignore
    }
    throw new Error(message);
  }
  if (res.status === 204) {
    // no content
    return {} as T;
  }
  return res.json() as Promise<T>;
}

export function App() {
  const [creatingUserName, setCreatingUserName] = useState("");
  const [createdUser, setCreatedUser] = useState<User | null>(null);
  const [depositAmount, setDepositAmount] = useState("1000");
  const [balance, setBalance] = useState<number | null>(null);
  const [orderAmount, setOrderAmount] = useState("500");
  const [order, setOrder] = useState<Order | null>(null);
  const [sellerId, setSellerId] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const buyerId = createdUser?.id;

  useEffect(() => {
    if (!buyerId) {
      setBalance(null);
      return;
    }
    (async () => {
      try {
        const data = await jsonFetch<{ balance: number }>(
          `${API_BASE}/users/${buyerId}/balance`
        );
        setBalance(data.balance);
      } catch {
        // ignore on first load
      }
    })();
  }, [buyerId]);

  const run = async (fn: () => Promise<void>) => {
    setLoading(true);
    setError(null);
    try {
      await fn();
    } catch (e: any) {
      setError(e.message ?? "Unknown error");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="app">
      <header className="header">
        <h1>Mini Escrow Console</h1>
        <p>Black &amp; White · Escrow flow playground</p>
      </header>

      <main className="grid">
        <section className="card">
          <h2>1. User 생성</h2>
          <label className="field">
            <span>Name</span>
            <input
              value={creatingUserName}
              onChange={(e) => setCreatingUserName(e.target.value)}
              placeholder="Alice"
            />
          </label>
          <button
            disabled={!creatingUserName || loading}
            onClick={() =>
              run(async () => {
                const user = await jsonFetch<User>(`${API_BASE}/users`, {
                  method: "POST",
                  body: JSON.stringify({ name: creatingUserName }),
                });
                setCreatedUser(user);
                setBalance(0);
              })
            }
          >
            Create User
          </button>
          {createdUser && (
            <p className="meta">
              Current user: <strong>#{createdUser.id}</strong> {createdUser.name}
            </p>
          )}
        </section>

        <section className="card">
          <h2>2. Deposit (충전)</h2>
          <p className="hint">
            유저를 먼저 만들고, 그 유저 ID 기준으로 ledger에 DEPOSIT을
            추가합니다.
          </p>
          <label className="field">
            <span>Amount</span>
            <input
              type="number"
              value={depositAmount}
              onChange={(e) => setDepositAmount(e.target.value)}
              min={1}
            />
          </label>
          <button
            disabled={!buyerId || loading}
            onClick={() =>
              run(async () => {
                if (!buyerId) return;
                await jsonFetch(`${API_BASE}/users/${buyerId}/deposit`, {
                  method: "POST",
                  body: JSON.stringify({ amount: Number(depositAmount) }),
                });
                const data = await jsonFetch<{ balance: number }>(
                  `${API_BASE}/users/${buyerId}/balance`
                );
                setBalance(data.balance);
              })
            }
          >
            Deposit to User
          </button>
          <div className="meta">
            <span>Balance</span>
            <strong>
              {balance !== null ? `${balance.toLocaleString()} 원` : "-"}
            </strong>
          </div>
        </section>

        <section className="card">
          <h2>3. Order 생성</h2>
          <p className="hint">
            buyer / seller / amount로 주문을 생성합니다. buyer는 위에서 만든
            유저를 그대로 사용해도 됩니다.
          </p>
          <div className="field-row">
            <label className="field">
              <span>Buyer ID</span>
              <input
                value={buyerId ?? ""}
                readOnly
                placeholder="자동 채움"
              />
            </label>
            <label className="field">
              <span>Seller ID</span>
              <input
                value={sellerId}
                onChange={(e) => setSellerId(e.target.value)}
                placeholder="예: 2"
              />
            </label>
          </div>
          <label className="field">
            <span>Amount</span>
            <input
              type="number"
              value={orderAmount}
              onChange={(e) => setOrderAmount(e.target.value)}
              min={1}
            />
          </label>
          <button
            disabled={!buyerId || !sellerId || loading}
            onClick={() =>
              run(async () => {
                if (!buyerId) return;
                const o = await jsonFetch<Order>(`${API_BASE}/orders`, {
                  method: "POST",
                  body: JSON.stringify({
                    buyer_id: buyerId,
                    seller_id: Number(sellerId),
                    amount: Number(orderAmount),
                  }),
                });
                setOrder(o);
              })
            }
          >
            Create Order
          </button>
          {order && (
            <div className="meta">
              <div>
                Order #{order.id} · {order.amount.toLocaleString()} 원
              </div>
              <div>Status: {order.status}</div>
            </div>
          )}
        </section>

        <section className="card flow">
          <h2>4. Escrow Flow</h2>
          <p className="hint">
            CREATED → FUNDED → CONFIRMED / CANCELLED 흐름을 직접 눌러보면서
            테스트할 수 있습니다.
          </p>
          <div className="flow-buttons">
            <button
              disabled={!order || loading}
              onClick={() =>
                run(async () => {
                  if (!order) return;
                  await jsonFetch(
                    `${API_BASE}/orders/${order.id}/fund`,
                    { method: "POST" }
                  );
                  setOrder({ ...order, status: "FUNDED" });
                })
              }
            >
              Fund
            </button>
            <button
              disabled={!order || loading}
              onClick={() =>
                run(async () => {
                  if (!order) return;
                  await jsonFetch(
                    `${API_BASE}/orders/${order.id}/confirm`,
                    { method: "POST" }
                  );
                  setOrder({ ...order, status: "CONFIRMED" });
                })
              }
            >
              Confirm
            </button>
            <button
              disabled={!order || loading}
              onClick={() =>
                run(async () => {
                  if (!order) return;
                  await jsonFetch(
                    `${API_BASE}/orders/${order.id}/cancel`,
                    { method: "POST" }
                  );
                  setOrder({ ...order, status: "CANCELLED" });
                })
              }
            >
              Cancel
            </button>
          </div>
          {order && (
            <div className="meta">
              현재 상태: <strong>{order.status}</strong>
            </div>
          )}
        </section>
      </main>

      {error && (
        <div className="toast error">
          <span>{error}</span>
        </div>
      )}
      {loading && (
        <div className="toast info">
          <span>Processing...</span>
        </div>
      )}
    </div>
  );
}

