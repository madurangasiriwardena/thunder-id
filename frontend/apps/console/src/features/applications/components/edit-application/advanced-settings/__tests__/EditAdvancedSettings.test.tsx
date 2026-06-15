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

import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, it, expect, vi} from 'vitest';
import CertificateTypes from '../../../../constants/certificate-types';
import type {Application} from '../../../../models/application';
import type {OAuth2Config} from '../../../../models/oauth';
import EditAdvancedSettings from '../EditAdvancedSettings';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

// EditAdvancedSettings loads the deployment session groups for the SSO Session Group dropdown.
// Stub the hook so the component renders without the ConfigProvider/QueryClient providers.
vi.mock('../../../../api/useGetSessionGroups', () => ({
  default: () => ({data: {groups: [{id: 'g1', name: 'Group One'}]}}),
}));

describe('EditAdvancedSettings', () => {
  const mockApplication: Application = {
    id: 'test-app-id',
    name: 'Test Application',
    description: 'Test Description',
    template: 'custom',
    certificate: {
      type: CertificateTypes.NONE,
      value: '',
    },
    createdAt: '2025-01-01T00:00:00Z',
    updatedAt: '2025-01-15T00:00:00Z',
  } as Application;

  const mockOAuth2Config: OAuth2Config = {
    grantTypes: ['authorization_code', 'refresh_token'],
    responseTypes: ['code'],
    pkceRequired: true,
    publicClient: false,
  };

  const mockOnFieldChange = vi.fn();

  describe('Rendering', () => {
    it('should render all three sections', () => {
      render(
        <EditAdvancedSettings
          application={mockApplication}
          editedApp={{}}
          oauth2Config={mockOAuth2Config}
          onFieldChange={mockOnFieldChange}
        />,
      );

      expect(screen.getByText('applications:edit.advanced.labels.oauth2Config')).toBeInTheDocument();
      expect(screen.getByText('applications:edit.advanced.labels.certificate')).toBeInTheDocument();
      expect(screen.getByText('applications:edit.advanced.labels.metadata')).toBeInTheDocument();
    });

    it('should render without OAuth2 config when not provided', () => {
      render(<EditAdvancedSettings application={mockApplication} editedApp={{}} onFieldChange={mockOnFieldChange} />);

      expect(screen.queryByText('applications:edit.advanced.labels.oauth2Config')).not.toBeInTheDocument();
      expect(screen.getByText('applications:edit.advanced.labels.certificate')).toBeInTheDocument();
      expect(screen.getByText('applications:edit.advanced.labels.metadata')).toBeInTheDocument();
    });

    it('should render without metadata section when timestamps are missing', () => {
      const appWithoutMetadata = {...mockApplication};
      delete (appWithoutMetadata as Partial<Application>).createdAt;
      delete (appWithoutMetadata as Partial<Application>).updatedAt;

      render(
        <EditAdvancedSettings
          application={appWithoutMetadata}
          editedApp={{}}
          oauth2Config={mockOAuth2Config}
          onFieldChange={mockOnFieldChange}
        />,
      );

      expect(screen.getByText('applications:edit.advanced.labels.oauth2Config')).toBeInTheDocument();
      expect(screen.getByText('applications:edit.advanced.labels.certificate')).toBeInTheDocument();
      expect(screen.queryByText('applications:edit.advanced.labels.metadata')).not.toBeInTheDocument();
    });
  });

  describe('Section Integration', () => {
    it('should pass correct props to OAuth2ConfigSection', () => {
      render(
        <EditAdvancedSettings
          application={mockApplication}
          editedApp={{}}
          oauth2Config={mockOAuth2Config}
          onFieldChange={mockOnFieldChange}
        />,
      );

      expect(screen.getByText('authorization_code')).toBeInTheDocument();
      expect(screen.getByText('refresh_token')).toBeInTheDocument();
      expect(screen.getByText('code')).toBeInTheDocument();
    });

    it('should pass correct props to CertificateSection', () => {
      render(
        <EditAdvancedSettings
          application={mockApplication}
          editedApp={{}}
          oauth2Config={mockOAuth2Config}
          onFieldChange={mockOnFieldChange}
        />,
      );

      expect(screen.getByLabelText('applications:edit.advanced.labels.certificateType')).toBeInTheDocument();
    });

    it('should pass correct props to MetadataSection', () => {
      render(
        <EditAdvancedSettings
          application={mockApplication}
          editedApp={{}}
          oauth2Config={mockOAuth2Config}
          onFieldChange={mockOnFieldChange}
        />,
      );

      expect(screen.getByText('applications:edit.advanced.labels.createdAt')).toBeInTheDocument();
      expect(screen.getByText('applications:edit.advanced.labels.updatedAt')).toBeInTheDocument();
    });
  });

  describe('SSO Session Group', () => {
    it('includes the selected session group in the inboundAuthConfig payload', async () => {
      const user = userEvent.setup();
      const onFieldChange = vi.fn();
      const appWithOAuth = {
        ...mockApplication,
        inboundAuthConfig: [{type: 'oauth2', config: mockOAuth2Config}],
      } as Application;

      render(
        <EditAdvancedSettings
          application={appWithOAuth}
          editedApp={{}}
          oauth2Config={mockOAuth2Config}
          onFieldChange={onFieldChange}
        />,
      );

      // The session group select shows the "OU default group" placeholder until a group is chosen.
      await user.click(screen.getByText('applications:edit.advanced.sessionGroup.defaultOption'));
      await user.click(await screen.findByRole('option', {name: 'Group One'}));

      // The change must carry sessionGroupId on the oauth2 inbound config so it is included in
      // the PUT payload assembled by the edit page (which spreads editedApp verbatim).
      const lastCall = onFieldChange.mock.calls.find(([field]) => field === 'inboundAuthConfig');
      expect(lastCall).toBeDefined();
      const updatedConfigs = lastCall?.[1] as {type: string; config: {sessionGroupId?: string}}[];
      const oauthEntry = updatedConfigs.find((c) => c.type === 'oauth2');
      expect(oauthEntry?.config.sessionGroupId).toBe('g1');
    });
  });

  describe('Layout', () => {
    it('should render sections in a Stack with spacing', () => {
      const {container} = render(
        <EditAdvancedSettings
          application={mockApplication}
          editedApp={{}}
          oauth2Config={mockOAuth2Config}
          onFieldChange={mockOnFieldChange}
        />,
      );

      const stack = container.firstChild;
      expect(stack).toHaveClass('MuiStack-root');
    });
  });

  describe('Edge Cases', () => {
    it('should handle undefined oauth2Config', () => {
      render(
        <EditAdvancedSettings
          application={mockApplication}
          editedApp={{}}
          oauth2Config={undefined}
          onFieldChange={mockOnFieldChange}
        />,
      );

      expect(screen.queryByText('applications:edit.advanced.labels.oauth2Config')).not.toBeInTheDocument();
    });

    it('should handle empty editedApp', () => {
      render(
        <EditAdvancedSettings
          application={mockApplication}
          editedApp={{}}
          oauth2Config={mockOAuth2Config}
          onFieldChange={mockOnFieldChange}
        />,
      );

      expect(screen.getByText('applications:edit.advanced.labels.certificate')).toBeInTheDocument();
    });

    it('should render with minimal application data', () => {
      const minimalApp = {
        id: 'minimal-id',
        name: 'Minimal App',
        template: 'custom',
      } as Application;

      render(<EditAdvancedSettings application={minimalApp} editedApp={{}} onFieldChange={mockOnFieldChange} />);

      expect(screen.getByText('applications:edit.advanced.labels.certificate')).toBeInTheDocument();
    });
  });
});
