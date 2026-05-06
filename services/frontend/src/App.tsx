import { Route, Routes, Navigate } from 'react-router-dom';
import LoginPage from './routes/LoginPage';
import OrdersPage from './routes/OrdersPage';
import HealthPage from './routes/HealthPage';
import TradePage from './routes/TradePage';
import { AppShell } from './components/layout/AppShell';

export default function App() {
  return (
    <AppShell>
      <Routes>
        <Route path="/" element={<Navigate to="/trade/BTC-KRW" replace />} />
        <Route path="/trade/:pair" element={<TradePage />} />
        <Route path="/login" element={<LoginPage />} />
        <Route path="/orders" element={<OrdersPage />} />
        <Route path="/health" element={<HealthPage />} />
      </Routes>
    </AppShell>
  );
}
