import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';

export const getPageStyles = (theme: GrafanaTheme2) => ({
  page: css`
    display: grid;
    gap: ${theme.spacing(2)};
    min-width: 0;
  `,
  header: css`
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: ${theme.spacing(2)};
    min-width: 0;

    @media (max-width: 700px) {
      flex-direction: column;
      align-items: stretch;
    }

    > * {
      min-width: 0;
    }
  `,
  eyebrow: css`
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.bodySmall.fontSize};
    margin-bottom: ${theme.spacing(0.5)};
  `,
  title: css`
    margin: 0;
    font-size: 24px;
    font-weight: ${theme.typography.fontWeightMedium};
  `,
  subtitle: css`
    margin-top: ${theme.spacing(0.5)};
    color: ${theme.colors.text.secondary};
  `,
  toolbar: css`
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: ${theme.spacing(1)};
    min-width: 0;
  `,
  grid4: css`
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: ${theme.spacing(2)};

    @media (max-width: 1100px) {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    @media (max-width: 700px) {
      grid-template-columns: minmax(0, 1fr);
    }
  `,
  grid2: css`
    display: grid;
    grid-template-columns: minmax(0, 1.15fr) minmax(360px, 0.85fr);
    gap: ${theme.spacing(2)};

    @media (max-width: 1000px) {
      grid-template-columns: minmax(0, 1fr);
    }
  `,
  split: css`
    display: grid;
    grid-template-columns: minmax(0, 1fr) 440px;
    gap: ${theme.spacing(2)};

    @media (max-width: 980px) {
      grid-template-columns: minmax(0, 1fr);
    }
  `,
  card: css`
    background: ${theme.colors.background.primary};
    border: 1px solid ${theme.colors.border.weak};
    border-radius: ${theme.shape.radius.default};
    padding: ${theme.spacing(2)};
    min-width: 0;
  `,
  metricLabel: css`
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.bodySmall.fontSize};
  `,
  metricValue: css`
    margin-top: ${theme.spacing(1)};
    font-size: 28px;
    line-height: 1.15;
    font-weight: ${theme.typography.fontWeightBold};
  `,
  table: css`
    width: 100%;
    border-collapse: collapse;
    font-size: ${theme.typography.bodySmall.fontSize};
    min-width: 0;
    th {
      text-align: left;
      color: ${theme.colors.text.secondary};
      background: ${theme.colors.background.secondary};
      border-bottom: 1px solid ${theme.colors.border.weak};
      padding: ${theme.spacing(1)};
      font-weight: ${theme.typography.fontWeightMedium};
    }
    td {
      border-bottom: 1px solid ${theme.colors.border.weak};
      padding: ${theme.spacing(1)};
      vertical-align: middle;
    }

    @media (max-width: 700px) {
      display: block;
      overflow-x: auto;
      white-space: nowrap;
    }
  `,
  formGrid: css`
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: ${theme.spacing(2)};

    @media (max-width: 700px) {
      grid-template-columns: minmax(0, 1fr);
    }
  `,
  fullSpan: css`
    grid-column: 1 / -1;
  `,
  field: css`
    min-width: 0;
  `,
  fieldLabel: css`
    display: inline-flex;
    align-items: center;
    gap: ${theme.spacing(0.5)};
    margin-bottom: ${theme.spacing(0.75)};
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.bodySmall.fontSize};
    min-height: 18px;
  `,
  helpIcon: css`
    color: ${theme.colors.text.secondary};
    opacity: 0.85;

    &:hover,
    &:focus-visible {
      color: ${theme.colors.text.primary};
      opacity: 1;
    }
  `,
  helpPopover: css`
    max-width: 340px;
    padding: ${theme.spacing(1.5)};
    color: ${theme.colors.text.primary};
    background: ${theme.colors.background.elevated};
    border: 1px solid ${theme.colors.border.weak};
    border-radius: ${theme.shape.radius.default};
    box-shadow: ${theme.shadows.z3};
    font-size: ${theme.typography.bodySmall.fontSize};
    line-height: 1.45;
    overflow-wrap: anywhere;
  `,
  input: css`
    width: 100%;
    min-height: 34px;
    border: 1px solid ${theme.colors.border.medium};
    border-radius: ${theme.shape.radius.default};
    color: ${theme.colors.text.primary};
    background: ${theme.colors.background.primary};
    padding: 6px 9px;
  `,
  checkboxLine: css`
    display: flex;
    align-items: center;
    gap: ${theme.spacing(1)};
    height: 34px;
  `,
  code: css`
    margin: 0;
    min-height: 360px;
    max-height: 620px;
    max-width: 100%;
    overflow: auto;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    color: #dbeafe;
    background: #0f172a;
    border-radius: ${theme.shape.radius.default};
    padding: ${theme.spacing(2)};
    font-family: ${theme.typography.fontFamilyMonospace};
    font-size: 12px;
    line-height: 1.55;
  `,
  mono: css`
    font-family: ${theme.typography.fontFamilyMonospace};
    overflow-wrap: anywhere;
  `,
  chip: css`
    display: inline-flex;
    align-items: center;
    height: 22px;
    padding: 0 8px;
    border-radius: 999px;
    background: ${theme.colors.background.secondary};
    color: ${theme.colors.text.primary};
    font-size: 12px;
    white-space: nowrap;
  `,
  chipGreen: css`
    color: ${theme.colors.success.text};
    background: ${theme.colors.success.transparent};
  `,
  chipRed: css`
    color: ${theme.colors.error.text};
    background: ${theme.colors.error.transparent};
  `,
  chipBlue: css`
    color: ${theme.colors.info.text};
    background: ${theme.colors.info.transparent};
  `,
  chipOrange: css`
    color: ${theme.colors.warning.text};
    background: ${theme.colors.warning.transparent};
  `,
  rowActions: css`
    display: flex;
    flex-wrap: wrap;
    gap: ${theme.spacing(0.75)};
  `,
  muted: css`
    color: ${theme.colors.text.secondary};
  `,
  danger: css`
    color: ${theme.colors.error.text};
  `,
  drawer: css`
    border-left: 1px solid ${theme.colors.border.weak};
    background: ${theme.colors.background.primary};
    padding: ${theme.spacing(2)};
  `,
  message: css`
    border-left: 3px solid ${theme.colors.info.border};
    background: ${theme.colors.info.transparent};
    padding: ${theme.spacing(1.5)};
    border-radius: ${theme.shape.radius.default};
    overflow-wrap: anywhere;
  `,
  error: css`
    border-left: 3px solid ${theme.colors.error.border};
    background: ${theme.colors.error.transparent};
    padding: ${theme.spacing(1.5)};
    border-radius: ${theme.shape.radius.default};
    overflow-wrap: anywhere;
  `,
});
