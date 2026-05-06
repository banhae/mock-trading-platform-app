import { useEffect, useState } from 'react';

// 가벼운 toast 알림 시스템. 외부 라이브러리 없이 module singleton +
// 구독으로 동작한다. 사용처에서는 pushToast() 를 호출하면 된다.
//
// - tone 은 성공/실패/안내 3가지로만 제한한다.
// - 자동으로 닫히고, 사용자가 직접 닫을 수도 있다.
// - SSR / 테스트 환경에서도 안전하게 import 가능해야 한다.

export type ToastTone = 'success' | 'error' | 'info';

export interface Toast {
  id: number;
  tone: ToastTone;
  message: string;
}

export interface PushToastInput {
  tone: ToastTone;
  message: string;
  durationMs?: number;
}

type Listener = (toasts: Toast[]) => void;

const DEFAULT_DURATION_MS = 3200;

let nextId = 1;
let toasts: Toast[] = [];
const listeners = new Set<Listener>();

function emit(): void {
  for (const listener of listeners) {
    listener(toasts);
  }
}

export function pushToast(input: PushToastInput): number {
  const id = nextId++;
  const toast: Toast = { id, tone: input.tone, message: input.message };
  toasts = [...toasts, toast];
  emit();

  const duration = input.durationMs ?? DEFAULT_DURATION_MS;
  if (typeof window !== 'undefined' && duration > 0) {
    window.setTimeout(() => dismissToast(id), duration);
  }
  return id;
}

export function dismissToast(id: number): void {
  const next = toasts.filter((t) => t.id !== id);
  if (next.length === toasts.length) return;
  toasts = next;
  emit();
}

export function useToasts(): Toast[] {
  const [state, setState] = useState<Toast[]>(toasts);
  useEffect(() => {
    const listener: Listener = (next) => setState(next);
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  }, []);
  return state;
}

export function ToastHub() {
  const items = useToasts();
  if (items.length === 0) return null;
  return (
    <div className="toast-hub" role="status" aria-live="polite">
      {items.map((toast) => (
        <div key={toast.id} className={`toast toast--${toast.tone}`}>
          <span className="toast__dot" aria-hidden="true" />
          <span className="toast__message">{toast.message}</span>
          <button
            type="button"
            className="toast__close"
            aria-label="Dismiss"
            onClick={() => dismissToast(toast.id)}
          >
            ×
          </button>
        </div>
      ))}
    </div>
  );
}

