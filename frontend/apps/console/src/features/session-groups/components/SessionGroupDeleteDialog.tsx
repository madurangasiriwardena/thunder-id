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

import {Dialog, DialogTitle, DialogContent, DialogContentText, DialogActions, Button, Alert} from '@wso2/oxygen-ui';
import {useState, type JSX} from 'react';
import {useTranslation} from 'react-i18next';
import useDeleteSessionGroup from '../api/useDeleteSessionGroup';

export interface SessionGroupDeleteDialogProps {
  open: boolean;
  sessionGroupId: string | null;
  onClose: () => void;
  onSuccess?: () => void;
}

/**
 * Dialog component for confirming session group deletion.
 */
export default function SessionGroupDeleteDialog({
  open,
  sessionGroupId,
  onClose,
  onSuccess = undefined,
}: SessionGroupDeleteDialogProps): JSX.Element {
  const {t} = useTranslation();
  const deleteSessionGroup = useDeleteSessionGroup();
  const [error, setError] = useState<string | null>(null);

  const handleCancel = (): void => {
    if (deleteSessionGroup.isPending) return;
    setError(null);
    onClose();
  };

  const handleConfirm = (): void => {
    if (!sessionGroupId) return;

    setError(null);
    deleteSessionGroup.mutate(sessionGroupId, {
      onSuccess: (): void => {
        setError(null);
        onClose();
        onSuccess?.();
      },
      onError: (err: Error) => {
        setError(err.message ?? t('sessionGroups:delete.error'));
      },
    });
  };

  return (
    <Dialog open={open} onClose={handleCancel} maxWidth="sm" fullWidth>
      <DialogTitle>{t('sessionGroups:delete.title')}</DialogTitle>
      <DialogContent>
        <DialogContentText sx={{mb: 2}}>{t('sessionGroups:delete.message')}</DialogContentText>
        <Alert severity="warning" sx={{mb: 2}}>
          {t('sessionGroups:delete.disclaimer')}
        </Alert>
        {error && (
          <Alert severity="error" sx={{mt: 2}}>
            {error}
          </Alert>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={handleCancel} disabled={deleteSessionGroup.isPending}>
          {t('common:actions.cancel')}
        </Button>
        <Button
          onClick={handleConfirm}
          color="error"
          variant="contained"
          disabled={deleteSessionGroup.isPending || !sessionGroupId}
        >
          {deleteSessionGroup.isPending ? t('common:status.deleting') : t('common:actions.delete')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
