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

import {Stack, Button, PageContent, PageTitle} from '@wso2/oxygen-ui';
import {Plus} from '@wso2/oxygen-ui-icons-react';
import {useState, type JSX} from 'react';
import {useTranslation} from 'react-i18next';
import {Link} from 'react-router';
import CreateSessionGroupDialog from '../components/CreateSessionGroupDialog';
import SessionGroupsList from '../components/SessionGroupsList';

export default function SessionGroupsListPage(): JSX.Element {
  const {t} = useTranslation();
  const [createDialogOpen, setCreateDialogOpen] = useState<boolean>(false);

  return (
    <PageContent>
      <PageTitle>
        <PageTitle.BackButton component={<Link to="/applications" />}>
          {t('sessionGroups:listing.backToApplications')}
        </PageTitle.BackButton>
        <PageTitle.Header>{t('sessionGroups:listing.title')}</PageTitle.Header>
        <PageTitle.SubHeader>{t('sessionGroups:listing.subtitle')}</PageTitle.SubHeader>
        <PageTitle.Actions>
          <Stack direction="row" spacing={2}>
            <Button
              data-testid="session-group-add-button"
              variant="contained"
              startIcon={<Plus size={18} />}
              onClick={() => setCreateDialogOpen(true)}
            >
              {t('sessionGroups:listing.addSessionGroup')}
            </Button>
          </Stack>
        </PageTitle.Actions>
      </PageTitle>

      <SessionGroupsList />

      <CreateSessionGroupDialog open={createDialogOpen} onClose={() => setCreateDialogOpen(false)} />
    </PageContent>
  );
}
