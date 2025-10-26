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

import React, { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import CardPro from '../../common/ui/CardPro';
import LogsTable from './UsageLogsTable';
import LogsActions from './UsageLogsActions';
import LogsFilters from './UsageLogsFilters';
import ColumnSelectorModal from './modals/ColumnSelectorModal';
import UserInfoModal from './modals/UserInfoModal';
import { useLogsData } from '../../../hooks/usage-logs/useUsageLogsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

const LogsPage = () => {
  const logsData = useLogsData();
  const isMobile = useIsMobile();
  const location = useLocation();

  // Handle URL parameters for channel ID
  useEffect(() => {
    if (logsData.formApi) {
      const searchParams = new URLSearchParams(location.search);
      const channelParam = searchParams.get('channel');

      if (channelParam) {
        // Set channel ID in form
        logsData.formApi.setValue('channel', channelParam);
        // Trigger search after a short delay to ensure form is updated
        setTimeout(() => {
          logsData.refresh();
        }, 100);
      }
    }
  }, [location.search, logsData.formApi, logsData.refresh]);

  return (
    <>
      {/* Modals */}
      <ColumnSelectorModal {...logsData} />
      <UserInfoModal {...logsData} />

      {/* Main Content */}
      <CardPro
        type='type2'
        statsArea={<LogsActions {...logsData} />}
        searchArea={<LogsFilters {...logsData} />}
        paginationArea={createCardProPagination({
          currentPage: logsData.activePage,
          pageSize: logsData.pageSize,
          total: logsData.logCount,
          onPageChange: logsData.handlePageChange,
          onPageSizeChange: logsData.handlePageSizeChange,
          isMobile: isMobile,
          t: logsData.t,
        })}
        t={logsData.t}
      >
        <LogsTable {...logsData} />
      </CardPro>
    </>
  );
};

export default LogsPage;
