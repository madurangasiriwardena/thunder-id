/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {useDataGridLocaleText} from '@thunderid/hooks';
import {useLogger} from '@thunderid/logger/react';
import {Box, Chip, IconButton, Typography, Tooltip, DataGrid, ListingTable} from '@wso2/oxygen-ui';
import {Pencil, Trash2} from '@wso2/oxygen-ui-icons-react';
import {useMemo, useCallback, useState, type JSX} from 'react';
import {useTranslation} from 'react-i18next';
import {useNavigate} from 'react-router';
import SessionGroupDeleteDialog from './SessionGroupDeleteDialog';
import useGetSessionGroups from '../api/useGetSessionGroups';
import type {SessionGroup} from '../models/session-group';

/**
 * DataGrid component for displaying the list of SSO session groups.
 */
export default function SessionGroupsList(): JSX.Element {
  const navigate = useNavigate();
  const {t} = useTranslation();
  const logger = useLogger('SessionGroupsList');
  const dataGridLocaleText = useDataGridLocaleText();

  const {data, isLoading, error} = useGetSessionGroups();

  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState<boolean>(false);

  const handleEditClick = useCallback(
    (groupId: string): void => {
      (async (): Promise<void> => {
        await navigate(`/session-groups/${groupId}`);
      })().catch((_error: unknown) => {
        logger.error('Failed to navigate to session group details', {error: _error, groupId});
      });
    },
    [navigate, logger],
  );

  const handleDeleteClick = useCallback((groupId: string): void => {
    setSelectedGroupId(groupId);
    setDeleteDialogOpen(true);
  }, []);

  const handleDeleteDialogClose = (): void => {
    setDeleteDialogOpen(false);
    setSelectedGroupId(null);
  };

  const columns: DataGrid.GridColDef<SessionGroup>[] = useMemo(
    () => [
      {
        field: 'name',
        headerName: t('sessionGroups:listing.columns.name'),
        flex: 1,
        minWidth: 200,
        renderCell: (params: DataGrid.GridRenderCellParams<SessionGroup>): JSX.Element => (
          <Box sx={{display: 'flex', alignItems: 'center', gap: 1}}>
            <Typography variant="body2">{params.row.name}</Typography>
            {params.row.isDefault && (
              <Chip label={t('sessionGroups:listing.defaultBadge')} size="small" color="primary" variant="outlined" />
            )}
          </Box>
        ),
      },
      {
        field: 'ouId',
        headerName: t('sessionGroups:listing.columns.organizationUnit'),
        flex: 1,
        minWidth: 200,
        renderCell: (params: DataGrid.GridRenderCellParams<SessionGroup>) => (
          <Typography variant="body2" sx={{fontFamily: 'monospace', fontSize: '0.875rem'}}>
            {params.row.ouId || '-'}
          </Typography>
        ),
      },
      {
        field: 'sessionMode',
        headerName: t('sessionGroups:listing.columns.mode'),
        width: 160,
        renderCell: (params: DataGrid.GridRenderCellParams<SessionGroup>): JSX.Element => (
          <Typography variant="body2">{t(`sessionGroups:mode.${params.row.sessionMode}`)}</Typography>
        ),
      },
      {
        field: 'actions',
        headerName: t('sessionGroups:listing.columns.actions'),
        width: 150,
        align: 'center',
        headerAlign: 'center',
        sortable: false,
        filterable: false,
        hideable: false,
        renderCell: (params: DataGrid.GridRenderCellParams<SessionGroup>): JSX.Element => (
          <ListingTable.RowActions>
            <Tooltip title={t('common:actions.edit')}>
              <IconButton
                size="small"
                aria-label={t('common:actions.edit')}
                onClick={(e) => {
                  e.stopPropagation();
                  handleEditClick(params.row.id);
                }}
              >
                <Pencil size={16} />
              </IconButton>
            </Tooltip>
            <Tooltip
              title={
                params.row.isDefault
                  ? t('sessionGroups:listing.deleteDefaultDisabled')
                  : t('common:actions.delete')
              }
            >
              <span>
                <IconButton
                  size="small"
                  color="error"
                  aria-label={t('common:actions.delete')}
                  disabled={params.row.isDefault}
                  onClick={(e) => {
                    e.stopPropagation();
                    handleDeleteClick(params.row.id);
                  }}
                >
                  <Trash2 size={16} />
                </IconButton>
              </span>
            </Tooltip>
          </ListingTable.RowActions>
        ),
      },
    ],
    [handleDeleteClick, handleEditClick, t],
  );

  if (error) {
    return (
      <Box sx={{textAlign: 'center', py: 8}}>
        <Typography variant="h6" color="error" gutterBottom>
          {t('sessionGroups:listing.error')}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {error.message ?? t('common:messages.somethingWentWrong')}
        </Typography>
      </Box>
    );
  }

  return (
    <>
      <ListingTable.Provider variant="data-grid-card" loading={isLoading}>
        <ListingTable.Container disablePaper>
          <ListingTable.DataGrid
            rows={data?.groups ?? []}
            columns={columns}
            getRowId={(row) => (row as SessionGroup).id}
            onRowClick={(params) => {
              handleEditClick((params.row as SessionGroup).id);
            }}
            disableRowSelectionOnClick
            localeText={dataGridLocaleText}
            autoHeight
            sx={{
              '& .MuiDataGrid-row': {cursor: 'pointer'},
            }}
          />
        </ListingTable.Container>
      </ListingTable.Provider>

      <SessionGroupDeleteDialog
        open={deleteDialogOpen}
        sessionGroupId={selectedGroupId}
        onClose={handleDeleteDialogClose}
      />
    </>
  );
}
