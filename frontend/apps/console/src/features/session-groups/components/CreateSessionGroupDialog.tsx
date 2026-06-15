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

import {OrganizationUnitTreePicker} from '@thunderid/configure-organization-units';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Alert,
  Stack,
  TextField,
  FormControl,
  FormLabel,
  Select,
  MenuItem,
  Typography,
} from '@wso2/oxygen-ui';
import {useState, type JSX} from 'react';
import {useTranslation} from 'react-i18next';
import useCreateSessionGroup from '../api/useCreateSessionGroup';
import {SESSION_MODES, type SessionMode} from '../models/session-group';

export interface CreateSessionGroupDialogProps {
  open: boolean;
  onClose: () => void;
  onSuccess?: (sessionGroupId: string) => void;
}

/**
 * Dialog for creating a new SSO session group (name, mode, owning organization unit).
 */
export default function CreateSessionGroupDialog({
  open,
  onClose,
  onSuccess = undefined,
}: CreateSessionGroupDialogProps): JSX.Element {
  const {t} = useTranslation();
  const createSessionGroup = useCreateSessionGroup();

  const [name, setName] = useState('');
  const [mode, setMode] = useState<SessionMode>('managed');
  const [ouId, setOuId] = useState('');
  const [error, setError] = useState<string | null>(null);

  const reset = (): void => {
    setName('');
    setMode('managed');
    setOuId('');
    setError(null);
  };

  const handleClose = (): void => {
    if (createSessionGroup.isPending) return;
    reset();
    onClose();
  };

  const isValid: boolean = name.trim().length > 0 && ouId.length > 0;

  const handleCreate = (): void => {
    if (!isValid) return;

    setError(null);
    createSessionGroup.mutate(
      {name: name.trim(), sessionMode: mode, ouId},
      {
        onSuccess: (group): void => {
          reset();
          onClose();
          onSuccess?.(group.id);
        },
        onError: (err: Error) => {
          setError(err.message ?? t('sessionGroups:create.error'));
        },
      },
    );
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('sessionGroups:create.title')}</DialogTitle>
      <DialogContent>
        <Stack direction="column" spacing={3} sx={{mt: 1}}>
          <FormControl fullWidth required>
            <FormLabel htmlFor="session-group-name">{t('sessionGroups:create.form.name.label')}</FormLabel>
            <TextField
              id="session-group-name"
              size="small"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('sessionGroups:create.form.name.placeholder')}
            />
          </FormControl>

          <FormControl fullWidth size="small">
            <FormLabel htmlFor="session-group-mode">{t('sessionGroups:create.form.mode.label')}</FormLabel>
            <Select
              id="session-group-mode"
              value={mode}
              onChange={(e) => setMode(e.target.value as SessionMode)}
            >
              {SESSION_MODES.map((m) => (
                <MenuItem key={m} value={m}>
                  {t(`sessionGroups:mode.${m}`)}
                </MenuItem>
              ))}
            </Select>
            <Typography variant="caption" color="text.secondary" sx={{mt: 0.5}}>
              {t('sessionGroups:create.form.mode.hint')}
            </Typography>
          </FormControl>

          <FormControl fullWidth required>
            <FormLabel>{t('sessionGroups:create.form.organizationUnit.label')}</FormLabel>
            <OrganizationUnitTreePicker
              id="session-group-create-ou-picker"
              value={ouId}
              onChange={setOuId}
              maxHeight={320}
            />
          </FormControl>

          {error && <Alert severity="error">{error}</Alert>}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={createSessionGroup.isPending}>
          {t('common:actions.cancel')}
        </Button>
        <Button onClick={handleCreate} variant="contained" disabled={!isValid || createSessionGroup.isPending}>
          {createSessionGroup.isPending ? t('sessionGroups:create.creating') : t('sessionGroups:create.submit')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
