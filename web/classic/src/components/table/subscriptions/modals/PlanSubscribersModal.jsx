/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useCallback, useEffect, useState } from 'react';
import {
  Modal,
  Table,
  Tag,
  Typography,
  Select,
  Progress,
  Space,
} from '@douyinfe/semi-ui';
import { API, showError, renderQuota } from '../../../../helpers';

const { Text } = Typography;

function formatTs(ts) {
  if (!ts) return '-';
  try {
    return new Date(ts * 1000).toLocaleString();
  } catch {
    return String(ts);
  }
}

function effectiveStatus(sub) {
  const now = Date.now() / 1000;
  if (sub?.status === 'cancelled') return 'cancelled';
  if (
    sub?.status === 'expired' ||
    ((sub?.end_time || 0) > 0 && sub.end_time < now)
  ) {
    return 'expired';
  }
  if (sub?.status === 'active') return 'active';
  return 'expired';
}

const PlanSubscribersModal = ({ visible, plan, onClose, t }) => {
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [status, setStatus] = useState('active');
  const planId = plan?.plan?.id;
  const planTitle = plan?.plan?.title || (planId ? `#${planId}` : '-');

  const load = useCallback(async () => {
    if (!planId) return;
    setLoading(true);
    try {
      const params = new URLSearchParams({
        p: String(page),
        page_size: String(pageSize),
        plan_id: String(planId),
      });
      if (status && status !== 'all') params.set('status', status);
      const res = await API.get(
        `/api/subscription/admin/user_subscriptions?${params.toString()}`,
      );
      if (res.data?.success) {
        setItems(res.data.data?.items || []);
        setTotal(res.data.data?.total || 0);
      } else {
        showError(res.data?.message || t('加载失败'));
      }
    } catch {
      showError(t('请求失败'));
    } finally {
      setLoading(false);
    }
  }, [planId, page, pageSize, status, t]);

  useEffect(() => {
    if (visible) {
      setPage(1);
      setStatus('active');
    }
  }, [visible, planId]);

  useEffect(() => {
    if (visible && planId) load();
  }, [visible, planId, load]);

  const columns = [
    {
      title: 'ID',
      width: 70,
      render: (_, row) => (
        <Text type='tertiary'>#{row?.subscription?.id}</Text>
      ),
    },
    {
      title: t('用户'),
      render: (_, row) => (
        <div>
          <Text strong>{row?.username || '-'}</Text>
          <Text type='tertiary' style={{ display: 'block' }}>
            ID: {row?.subscription?.user_id}
          </Text>
        </div>
      ),
    },
    {
      title: t('状态'),
      width: 90,
      render: (_, row) => {
        const s = effectiveStatus(row?.subscription);
        if (s === 'active') return <Tag color='green'>{t('有效')}</Tag>;
        if (s === 'cancelled') return <Tag color='grey'>{t('已作废')}</Tag>;
        return <Tag color='orange'>{t('已过期')}</Tag>;
      },
    },
    {
      title: t('额度用量'),
      width: 180,
      render: (_, row) => {
        const totalAmt = Number(row?.subscription?.amount_total || 0);
        const used = Number(row?.subscription?.amount_used || 0);
        if (totalAmt <= 0) {
          return (
            <div>
              <Text>{t('不限')}</Text>
              <Text type='tertiary' style={{ display: 'block' }}>
                {t('已用')}: {renderQuota(used)}
              </Text>
            </div>
          );
        }
        const pct = Math.min(100, Math.max(0, (used / totalAmt) * 100));
        return (
          <div>
            <Text>
              {renderQuota(used)} / {renderQuota(totalAmt)}
            </Text>
            <Progress percent={Number(pct.toFixed(0))} showInfo={false} />
          </div>
        );
      },
    },
    {
      title: t('有效期'),
      render: (_, row) => (
        <div>
          <Text type='tertiary' style={{ display: 'block' }}>
            {t('开始')}: {formatTs(row?.subscription?.start_time)}
          </Text>
          <Text type='tertiary' style={{ display: 'block' }}>
            {t('结束')}: {formatTs(row?.subscription?.end_time)}
          </Text>
        </div>
      ),
    },
    {
      title: t('来源'),
      width: 90,
      render: (_, row) => row?.subscription?.source || '-',
    },
  ];

  return (
    <Modal
      title={`${t('套餐订阅者')} · ${planTitle}`}
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={900}
      bodyStyle={{ maxHeight: '70vh', overflow: 'auto' }}
    >
      <Space style={{ marginBottom: 12 }}>
        <Select
          value={status}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
          style={{ width: 160 }}
        >
          <Select.Option value='all'>{t('全部状态')}</Select.Option>
          <Select.Option value='active'>{t('有效')}</Select.Option>
          <Select.Option value='expired'>{t('已过期')}</Select.Option>
          <Select.Option value='cancelled'>{t('已作废')}</Select.Option>
        </Select>
        <Text type='tertiary'>
          {t('共 {{count}} 条', { count: total })}
        </Text>
      </Space>
      <Table
        columns={columns}
        dataSource={items}
        loading={loading}
        rowKey={(row) => row?.subscription?.id}
        pagination={{
          currentPage: page,
          pageSize,
          total,
          onPageChange: setPage,
          onPageSizeChange: (size) => {
            setPageSize(size);
            setPage(1);
          },
        }}
      />
    </Modal>
  );
};

export default PlanSubscribersModal;
