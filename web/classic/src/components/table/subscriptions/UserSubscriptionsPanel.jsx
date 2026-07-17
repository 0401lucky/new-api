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
  Table,
  Tag,
  Typography,
  Select,
  Progress,
  Input,
  Space,
} from '@douyinfe/semi-ui';
import { API, showError, renderQuota } from '../../../helpers';

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

const UserSubscriptionsPanel = ({ plans = [], t }) => {
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [keyword, setKeyword] = useState('');
  const [keywordInput, setKeywordInput] = useState('');
  const [status, setStatus] = useState('all');
  const [planId, setPlanId] = useState(0);
  const [source, setSource] = useState('all');

  useEffect(() => {
    const timer = setTimeout(() => setKeyword(keywordInput.trim()), 300);
    return () => clearTimeout(timer);
  }, [keywordInput]);

  useEffect(() => {
    setPage(1);
  }, [keyword, status, planId, source]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        p: String(page),
        page_size: String(pageSize),
      });
      if (keyword) params.set('keyword', keyword);
      if (status && status !== 'all') params.set('status', status);
      if (planId > 0) params.set('plan_id', String(planId));
      if (source && source !== 'all') params.set('source', source);
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
  }, [page, pageSize, keyword, status, planId, source, t]);

  useEffect(() => {
    load();
  }, [load]);

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
      title: t('套餐'),
      render: (_, row) => (
        <div>
          <Text strong>
            {row?.plan_title || `#${row?.subscription?.plan_id}`}
          </Text>
          <Text type='tertiary' style={{ display: 'block' }}>
            plan #{row?.subscription?.plan_id}
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
      width: 200,
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
        const remaining = Math.max(0, totalAmt - used);
        return (
          <div>
            <Text>
              {renderQuota(used)} / {renderQuota(totalAmt)} ({pct.toFixed(0)}%)
            </Text>
            <Progress percent={Number(pct.toFixed(0))} showInfo={false} />
            <Text type='tertiary' style={{ display: 'block' }}>
              {t('剩余')}: {renderQuota(remaining)}
            </Text>
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
    <div>
      <Space wrap style={{ marginBottom: 12 }}>
        <Input
          value={keywordInput}
          onChange={setKeywordInput}
          placeholder={t('搜索用户 / 套餐 / ID')}
          style={{ width: 220 }}
          showClear
        />
        <Select
          value={status}
          onChange={setStatus}
          style={{ width: 140 }}
          placeholder={t('状态')}
        >
          <Select.Option value='all'>{t('全部状态')}</Select.Option>
          <Select.Option value='active'>{t('有效')}</Select.Option>
          <Select.Option value='expired'>{t('已过期')}</Select.Option>
          <Select.Option value='cancelled'>{t('已作废')}</Select.Option>
        </Select>
        <Select
          value={planId}
          onChange={setPlanId}
          style={{ width: 180 }}
          placeholder={t('套餐')}
        >
          <Select.Option value={0}>{t('全部套餐')}</Select.Option>
          {(plans || []).map((p) => (
            <Select.Option key={p?.plan?.id} value={p?.plan?.id}>
              {p?.plan?.title || `#${p?.plan?.id}`}
            </Select.Option>
          ))}
        </Select>
        <Select
          value={source}
          onChange={setSource}
          style={{ width: 140 }}
          placeholder={t('来源')}
        >
          <Select.Option value='all'>{t('全部来源')}</Select.Option>
          <Select.Option value='order'>{t('购买')}</Select.Option>
          <Select.Option value='admin'>{t('管理员')}</Select.Option>
          <Select.Option value='auto_grant'>{t('自动发放')}</Select.Option>
          <Select.Option value='balance'>{t('余额')}</Select.Option>
        </Select>
        <Text type='tertiary'>{t('共 {{count}} 条', { count: total })}</Text>
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
    </div>
  );
};

export default UserSubscriptionsPanel;
