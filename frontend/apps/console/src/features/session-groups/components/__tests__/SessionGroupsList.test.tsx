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

import userEvent from '@testing-library/user-event';
import {render, screen, waitFor} from '@thunderid/test-utils';
import type {NavigateFunction} from 'react-router';
import {describe, it, expect, beforeEach, vi} from 'vitest';
import type {SessionGroupListResponse} from '../../models/session-group';
import SessionGroupsList from '../SessionGroupsList';

const {mockLoggerError} = vi.hoisted(() => ({
  mockLoggerError: vi.fn(),
}));

vi.mock('../../api/useGetSessionGroups');
vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router');
  return {
    ...actual,
    useNavigate: vi.fn(),
  };
});
vi.mock('@thunderid/hooks', () => ({
  useDataGridLocaleText: vi.fn(),
}));

vi.mock('@wso2/oxygen-ui', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@wso2/oxygen-ui')>();
  return {
    ...actual,
    OxygenUIThemeProvider: ({children}: {children: React.ReactNode}) => children,
    ListingTable: {
      Provider: ({children}: {children: React.ReactNode}): React.ReactElement => children as React.ReactElement,
      Container: ({children}: {children: React.ReactNode}): React.ReactElement => children as React.ReactElement,
      DataGrid: ({
        rows,
        columns,
        onRowClick = undefined,
        getRowId,
      }: {
        rows: Record<string, unknown>[];
        columns: {
          field: string;
          renderCell?: (params: {row: Record<string, unknown>}) => React.ReactElement;
          valueGetter?: (value: unknown, row: Record<string, unknown>) => string;
        }[];
        onRowClick?: (params: {row: Record<string, unknown>}) => void;
        getRowId: (row: Record<string, unknown>) => string;
      }) => (
        <div role="grid" data-testid="data-grid">
          {rows.map((row: Record<string, unknown>) => (
            <div
              key={getRowId(row)}
              role="row"
              onClick={() => onRowClick?.({row})}
              onKeyDown={() => onRowClick?.({row})}
              tabIndex={0}
            >
              {columns.map((col) => {
                if (col.renderCell) {
                  return <div key={col.field}>{col.renderCell({row})}</div>;
                }
                if (col.valueGetter) {
                  return <div key={col.field}>{col.valueGetter(null, row)}</div>;
                }
                return <div key={col.field}>{row[col.field] as string}</div>;
              })}
            </div>
          ))}
        </div>
      ),
      RowActions: ({children}: {children: React.ReactNode}): React.ReactElement => children as React.ReactElement,
    },
  };
});

vi.mock('../SessionGroupDeleteDialog', () => ({
  default: ({open, onClose}: {open: boolean; onClose: () => void}) =>
    open ? (
      <div role="dialog" data-testid="delete-dialog">
        <button type="button" onClick={onClose}>
          Cancel
        </button>
      </div>
    ) : null,
}));

vi.mock('@thunderid/logger/react', () => ({
  useLogger: () => ({
    error: mockLoggerError,
    info: vi.fn(),
    warn: vi.fn(),
    debug: vi.fn(),
  }),
}));

const {default: useGetSessionGroups} = await import('../../api/useGetSessionGroups');
const {useNavigate} = await import('react-router');
const {useDataGridLocaleText} = await import('@thunderid/hooks');

describe('SessionGroupsList', () => {
  let mockNavigate: ReturnType<typeof vi.fn>;

  const mockData: SessionGroupListResponse = {
    totalResults: 2,
    groups: [
      {id: 'sg-1', name: 'Web Apps', ouId: 'ou-1', sessionMode: 'managed', isDefault: false},
      {id: 'sg-2', name: 'Default Group', ouId: 'ou-1', sessionMode: 'sessionless', isDefault: true},
    ],
  };

  beforeEach(() => {
    mockNavigate = vi.fn();
    mockLoggerError.mockReset();
    vi.mocked(useNavigate).mockReturnValue(mockNavigate as unknown as NavigateFunction);
    vi.mocked(useDataGridLocaleText).mockReturnValue({});
  });

  const renderComponent = () => render(<SessionGroupsList />);

  it('should render the error state', () => {
    vi.mocked(useGetSessionGroups).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Failed to load session groups'),
    } as ReturnType<typeof useGetSessionGroups>);

    renderComponent();

    expect(screen.getByText('Failed to load session groups')).toBeInTheDocument();
  });

  it('should render the session group rows', () => {
    vi.mocked(useGetSessionGroups).mockReturnValue({
      data: mockData,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useGetSessionGroups>);

    renderComponent();

    expect(screen.getByText('Web Apps')).toBeInTheDocument();
    expect(screen.getByText('Default Group')).toBeInTheDocument();
    // The default group's row exposes a disabled delete action (covered in detail below).
    expect(screen.getAllByRole('row')).toHaveLength(2);
  });

  it('should navigate to the edit page when a row is clicked', async () => {
    const user = userEvent.setup();
    vi.mocked(useGetSessionGroups).mockReturnValue({
      data: mockData,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useGetSessionGroups>);

    renderComponent();

    const rows = screen.getAllByRole('row');
    await user.click(rows[0]);

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/session-groups/sg-1');
    });
  });

  it('should disable delete for the default group and open the dialog for a non-default group', async () => {
    const user = userEvent.setup();
    vi.mocked(useGetSessionGroups).mockReturnValue({
      data: mockData,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useGetSessionGroups>);

    renderComponent();

    const deleteButtons = screen.getAllByRole('button', {name: /delete/i});
    // Two rows → two delete buttons; the default group's button is disabled.
    expect(deleteButtons.some((b) => (b as HTMLButtonElement).disabled)).toBe(true);

    const enabledDelete = deleteButtons.find((b) => !(b as HTMLButtonElement).disabled);
    expect(enabledDelete).toBeDefined();
    await user.click(enabledDelete!);

    await waitFor(() => {
      expect(screen.getByTestId('delete-dialog')).toBeInTheDocument();
    });
  });

  it('should render no rows when the list is empty', () => {
    vi.mocked(useGetSessionGroups).mockReturnValue({
      data: {totalResults: 0, groups: []},
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useGetSessionGroups>);

    renderComponent();

    expect(screen.getByRole('grid')).toBeInTheDocument();
    expect(screen.queryByRole('row')).not.toBeInTheDocument();
  });
});
