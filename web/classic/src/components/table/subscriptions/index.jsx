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

import React, { useContext, useEffect, useState } from 'react';
import { Banner, Tabs, TabPane } from '@douyinfe/semi-ui';
import CardPro from '../../common/ui/CardPro';
import SubscriptionsTable from './SubscriptionsTable';
import SubscriptionsActions from './SubscriptionsActions';
import SubscriptionsDescription from './SubscriptionsDescription';
import UserSubscriptionsPanel from './UserSubscriptionsPanel';
import AddEditSubscriptionModal from './modals/AddEditSubscriptionModal';
import PlanSubscribersModal from './modals/PlanSubscribersModal';
import { useSubscriptionsData } from '../../../hooks/subscriptions/useSubscriptionsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';
import { StatusContext } from '../../../context/Status';
import { API } from '../../../helpers';

const SubscriptionsPage = () => {
  const subscriptionsData = useSubscriptionsData();
  const isMobile = useIsMobile();
  const [statusState] = useContext(StatusContext);
  const enableEpay = !!statusState?.status?.enable_online_topup;
  const [complianceConfirmed, setComplianceConfirmed] = useState(true);
  const [activeTab, setActiveTab] = useState('plans');
  const [subscribersPlan, setSubscribersPlan] = useState(null);

  const {
    showEdit,
    editingPlan,
    sheetPlacement,
    closeEdit,
    refresh,
    openCreate,
    compactMode,
    setCompactMode,
    t,
    allPlans,
  } = subscriptionsData;

  const openPlanSubscribers = (planRecord) => {
    setSubscribersPlan(planRecord);
  };

  useEffect(() => {
    const loadComplianceStatus = async () => {
      try {
        const res = await API.get('/api/user/topup/info');
        if (res.data?.success) {
          setComplianceConfirmed(
            res.data.data?.payment_compliance_confirmed !== false,
          );
        }
      } catch (error) {
        // Keep the page usable if status loading fails; backend still enforces.
      }
    };
    loadComplianceStatus();
  }, []);

  return (
    <>
      <AddEditSubscriptionModal
        visible={showEdit}
        handleClose={closeEdit}
        editingPlan={editingPlan}
        placement={sheetPlacement}
        refresh={refresh}
        t={t}
      />
      <PlanSubscribersModal
        visible={!!subscribersPlan}
        plan={subscribersPlan}
        onClose={() => setSubscribersPlan(null)}
        t={t}
      />

      <CardPro
        type='type1'
        descriptionArea={
          activeTab === 'plans' ? (
            <SubscriptionsDescription
              compactMode={compactMode}
              setCompactMode={setCompactMode}
              t={t}
            />
          ) : null
        }
        actionsArea={
          <div className='flex flex-col md:flex-row justify-between items-start md:items-center gap-2 w-full'>
            <div className='order-1 md:order-0 w-full md:w-auto'>
              {activeTab === 'plans' ? (
                <SubscriptionsActions
                  openCreate={openCreate}
                  t={t}
                  disabled={!complianceConfirmed}
                />
              ) : null}
            </div>
            <Banner
              type='info'
              description={t('Stripe/Creem 需在第三方平台创建商品并填入 ID')}
              closeIcon={null}
              className='!rounded-lg order-2 md:order-1'
              style={{ maxWidth: '100%' }}
            />
          </div>
        }
        paginationArea={
          activeTab === 'plans'
            ? createCardProPagination({
                currentPage: subscriptionsData.activePage,
                pageSize: subscriptionsData.pageSize,
                total: subscriptionsData.planCount,
                onPageChange: subscriptionsData.handlePageChange,
                onPageSizeChange: subscriptionsData.handlePageSizeChange,
                isMobile,
                t: subscriptionsData.t,
              })
            : null
        }
        t={t}
      >
        {!complianceConfirmed && (
          <Banner
            type='warning'
            description={t(
              '订阅套餐创建和变更已锁定，管理员需先在支付设置中确认合规声明。',
            )}
            closeIcon={null}
            className='!rounded-lg mb-3'
          />
        )}
        <Tabs
          type='line'
          activeKey={activeTab}
          onChange={setActiveTab}
          className='mb-2'
        >
          <TabPane tab={t('套餐')} itemKey='plans' />
          <TabPane tab={t('用户订阅')} itemKey='instances' />
        </Tabs>
        {activeTab === 'plans' ? (
          <SubscriptionsTable
            {...subscriptionsData}
            openPlanSubscribers={openPlanSubscribers}
            enableEpay={enableEpay}
            complianceConfirmed={complianceConfirmed}
          />
        ) : (
          <UserSubscriptionsPanel plans={allPlans || []} t={t} />
        )}
      </CardPro>
    </>
  );
};

export default SubscriptionsPage;
