import { useState } from 'react';
import type { CreateOrderRequest } from '../api/orders';

export interface UseOrderTicketFormResult {
  values: CreateOrderRequest;
  createError: string;
  submitting: boolean;
  setField: (field: keyof CreateOrderRequest, value: string) => void;
  setCreateError: (value: string) => void;
  submit: (submitter: (payload: CreateOrderRequest) => Promise<void>) => Promise<void>;
}

export function useOrderTicketForm(): UseOrderTicketFormResult {
  const [values, setValues] = useState<CreateOrderRequest>({
    pair: 'BTC-KRW',
    side: 'buy',
    quantity: '0.01',
    price: '50000',
  });
  const [createError, setCreateError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  function setField(field: keyof CreateOrderRequest, value: string) {
    setValues((prev) => ({ ...prev, [field]: value }));
  }

  async function submit(
    submitter: (payload: CreateOrderRequest) => Promise<void>,
  ): Promise<void> {
    setCreateError('');
    setSubmitting(true);
    try {
      await submitter(values);
    } catch (err) {
      setCreateError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return {
    values,
    createError,
    submitting,
    setField,
    setCreateError,
    submit,
  };
}
