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

import {PageLoadingAnimation} from '@thunderid/components';
import {useToast} from '@thunderid/contexts';
import {useLogger} from '@thunderid/logger/react';
import {
  Box,
  Stack,
  Typography,
  Button,
  TextField,
  Paper,
  Alert,
  FormControl,
  FormLabel,
  Select,
  MenuItem,
  Divider,
  PageContent,
  PageTitle,
} from '@wso2/oxygen-ui';
import {ArrowLeft} from '@wso2/oxygen-ui-icons-react';
import {useState, useCallback, useMemo} from 'react';
import type {JSX} from 'react';
import {useTranslation} from 'react-i18next';
import {Link, useNavigate, useParams} from 'react-router';
import useGetSessionGroup from '../api/useGetSessionGroup';
import useUpdateSessionGroup from '../api/useUpdateSessionGroup';
import SessionGroupDeleteDialog from '../components/SessionGroupDeleteDialog';
import {SESSION_MODES, type SessionGroup, type SessionMode} from '../models/session-group';

export default function SessionGroupEditPage(): JSX.Element {
  const {sessionGroupId} = useParams<{sessionGroupId: string}>();
  const navigate = useNavigate();
  const {t} = useTranslation();
  const logger = useLogger('SessionGroupEditPage');
  const {showToast} = useToast();

  const {data: group, isLoading, error: fetchError, refetch} = useGetSessionGroup(sessionGroupId ?? '');
  const updateSessionGroup = useUpdateSessionGroup();

  const [edited, setEdited] = useState<Partial<SessionGroup>>({});
  const [deleteDialogOpen, setDeleteDialogOpen] = useState<boolean>(false);
  const listUrl = '/session-groups';

  const handleBack = async (): Promise<void> => {
    await navigate(listUrl);
  };

  const handleFieldChange = useCallback((field: keyof SessionGroup, value: unknown): void => {
    setEdited((prev) => ({...prev, [field]: value}));
  }, []);

  const hasChanges = useMemo(() => Object.keys(edited).length > 0, [edited]);

  const handleSave = useCallback(async (): Promise<void> => {
    if (!group || !sessionGroupId) return;

    try {
      await updateSessionGroup.mutateAsync({
        sessionGroupId,
        data: {
          name: edited.name ?? group.name,
          sessionMode: edited.sessionMode ?? group.sessionMode,
        },
      });
      setEdited({});
      await refetch();
    } catch (err: unknown) {
      logger.error('Failed to update session group', {error: err});
      const message = err instanceof Error ? err.message : t('sessionGroups:edit.saveError');
      showToast(message, 'error');
    }
  }, [group, sessionGroupId, edited, updateSessionGroup, refetch, logger, showToast, t]);

  const handleDeleteSuccess = (): void => {
    (async (): Promise<void> => {
      await navigate(listUrl);
    })().catch((_error: unknown) => {
      logger.error('Failed to navigate after deleting session group', {error: _error});
    });
  };

  if (isLoading) {
    return <PageLoadingAnimation />;
  }

  if (fetchError || !group) {
    return (
      <PageContent>
        <Alert severity={fetchError ? 'error' : 'warning'} sx={{mb: 2}}>
          {fetchError ? (fetchError.message ?? t('sessionGroups:edit.error')) : t('sessionGroups:edit.notFound')}
        </Alert>
        <Button
          onClick={() => {
            handleBack().catch((error: unknown) => {
              logger.error('Failed to navigate back', {error});
            });
          }}
          startIcon={<ArrowLeft size={16} />}
        >
          {t('sessionGroups:edit.back')}
        </Button>
      </PageContent>
    );
  }

  const effectiveName = edited.name ?? group.name;
  const effectiveMode = edited.sessionMode ?? group.sessionMode;

  return (
    <PageContent>
      <PageTitle>
        <PageTitle.BackButton component={<Link to={listUrl} />}>{t('sessionGroups:edit.back')}</PageTitle.BackButton>
        <PageTitle.Header>{group.name}</PageTitle.Header>
        <PageTitle.SubHeader>{t('sessionGroups:edit.subtitle')}</PageTitle.SubHeader>
      </PageTitle>

      <Paper variant="outlined" sx={{p: 3, maxWidth: 640}}>
        <Stack direction="column" spacing={3}>
          <FormControl fullWidth>
            <FormLabel htmlFor="session-group-name">{t('sessionGroups:edit.form.name.label')}</FormLabel>
            <TextField
              id="session-group-name"
              size="small"
              value={effectiveName}
              onChange={(e) => handleFieldChange('name', e.target.value)}
            />
          </FormControl>

          <FormControl fullWidth size="small">
            <FormLabel htmlFor="session-group-mode">{t('sessionGroups:edit.form.mode.label')}</FormLabel>
            <Select
              id="session-group-mode"
              value={effectiveMode}
              onChange={(e) => handleFieldChange('sessionMode', e.target.value as SessionMode)}
            >
              {SESSION_MODES.map((m) => (
                <MenuItem key={m} value={m}>
                  {t(`sessionGroups:mode.${m}`)}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          <FormControl fullWidth>
            <FormLabel>{t('sessionGroups:edit.form.organizationUnit.label')}</FormLabel>
            <Typography variant="body2" sx={{fontFamily: 'monospace', fontSize: '0.875rem'}}>
              {group.ouId}
            </Typography>
          </FormControl>

          {group.isDefault && (
            <Alert severity="info">{t('sessionGroups:edit.defaultNotice')}</Alert>
          )}

          <Divider />

          <Box>
            <Typography variant="subtitle2" color="error" gutterBottom>
              {t('sessionGroups:edit.dangerZone.title')}
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{mb: 2}}>
              {group.isDefault
                ? t('sessionGroups:edit.dangerZone.defaultDescription')
                : t('sessionGroups:edit.dangerZone.description')}
            </Typography>
            <Button
              variant="outlined"
              color="error"
              disabled={group.isDefault}
              onClick={() => setDeleteDialogOpen(true)}
            >
              {t('sessionGroups:edit.dangerZone.delete')}
            </Button>
          </Box>
        </Stack>
      </Paper>

      <SessionGroupDeleteDialog
        open={deleteDialogOpen}
        sessionGroupId={sessionGroupId ?? null}
        onClose={() => setDeleteDialogOpen(false)}
        onSuccess={handleDeleteSuccess}
      />

      {hasChanges && (
        <Paper
          sx={{
            position: 'fixed',
            bottom: 0,
            left: 0,
            right: 0,
            p: 2,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 2,
            borderRadius: '12px 12px 0 0',
            boxShadow: '0 -4px 20px rgba(0, 0, 0, 0.1)',
            zIndex: 1000,
            bgcolor: 'background.paper',
          }}
        >
          <Stack direction="row" spacing={2} alignItems="center">
            <Typography variant="body2">{t('sessionGroups:edit.unsavedChanges')}</Typography>
            <Button variant="outlined" color="error" onClick={() => setEdited({})}>
              {t('sessionGroups:edit.reset')}
            </Button>
            <Button
              variant="contained"
              onClick={() => {
                handleSave().catch(() => {
                  /* noop */
                });
              }}
              disabled={updateSessionGroup.isPending}
            >
              {updateSessionGroup.isPending ? t('sessionGroups:edit.saving') : t('sessionGroups:edit.save')}
            </Button>
          </Stack>
        </Paper>
      )}
    </PageContent>
  );
}
