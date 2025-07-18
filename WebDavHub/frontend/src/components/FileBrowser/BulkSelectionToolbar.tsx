import React from 'react';
import {
  Box,
  Collapse,
  Checkbox,
  Typography,
  Button,
  IconButton,
  Tooltip,
  alpha,
  useTheme,
} from '@mui/material';
import {
  Delete as DeleteIcon,
  Download as DownloadIcon,
  Close as CloseIcon,
  SelectAll as SelectAllIcon,
} from '@mui/icons-material';

interface BulkSelectionToolbarProps {
  selectedCount: number;
  totalCount: number;
  onSelectAll: (checked: boolean) => void;
  onClearSelection: () => void;
  onBulkDelete: () => void;
  onBulkDownload: () => void;
}

const BulkSelectionToolbar: React.FC<BulkSelectionToolbarProps> = ({
  selectedCount,
  totalCount,
  onSelectAll,
  onClearSelection,
  onBulkDelete,
  onBulkDownload,
}) => {
  const theme = useTheme();
  const isAllSelected = selectedCount === totalCount && totalCount > 0;
  const isIndeterminate = selectedCount > 0 && selectedCount < totalCount;

  return (
    <Collapse in={selectedCount > 0} timeout={300}>
      <Box sx={{ mb: 2 }}>
        <Box
          sx={{
            background: theme.palette.mode === 'dark'
              ? `linear-gradient(135deg, ${alpha(theme.palette.primary.main, 0.15)} 0%, ${alpha(theme.palette.primary.main, 0.08)} 100%)`
              : `linear-gradient(135deg, ${alpha(theme.palette.primary.main, 0.08)} 0%, ${alpha(theme.palette.primary.main, 0.04)} 100%)`,
            backdropFilter: 'blur(20px)',
            borderRadius: 3,
            border: theme.palette.mode === 'dark'
              ? `1px solid ${alpha(theme.palette.primary.main, 0.2)}`
              : `1px solid ${alpha(theme.palette.primary.main, 0.12)}`,
            boxShadow: theme.palette.mode === 'dark'
              ? `0 4px 20px ${alpha(theme.palette.primary.main, 0.1)}, 0 1px 4px ${alpha('#000', 0.2)}`
              : `0 4px 20px ${alpha(theme.palette.primary.main, 0.06)}, 0 1px 4px ${alpha('#000', 0.05)}`,
            p: 2.5,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            transition: 'all 0.2s ease-out',
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
            <Checkbox
              checked={isAllSelected}
              indeterminate={isIndeterminate}
              onChange={(e) => onSelectAll(e.target.checked)}
              size="small"
              sx={{
                p: 0,
                color: 'primary.main',
                '&.Mui-checked, &.MuiCheckbox-indeterminate': {
                  color: 'primary.main',
                },
              }}
            />
            <Typography
              variant="body2"
              sx={{
                fontWeight: 600,
                color: 'primary.main',
                fontSize: '0.875rem',
              }}
            >
              {isAllSelected
                ? `All ${totalCount} selected`
                : selectedCount > 0
                ? `${selectedCount} selected`
                : 'Select all'
              }
            </Typography>
          </Box>

          <Box sx={{ display: 'flex', gap: 1.5, alignItems: 'center' }}>
            {/* Quick select all button */}
            {!isAllSelected && totalCount > 0 && (
              <Tooltip title="Select all files">
                <Button
                  size="small"
                  startIcon={<SelectAllIcon />}
                  onClick={() => onSelectAll(true)}
                  sx={{
                    textTransform: 'none',
                    borderRadius: 2,
                    px: 2,
                  }}
                >
                  Select All ({totalCount})
                </Button>
              </Tooltip>
            )}

            {/* Bulk actions */}
            <Button
              variant="outlined"
              size="small"
              startIcon={<DownloadIcon />}
              onClick={onBulkDownload}
              disabled={selectedCount === 0}
              sx={{
                borderRadius: 2,
                textTransform: 'none',
                fontWeight: 600,
              }}
            >
              Download ({selectedCount})
            </Button>

            <Button
              variant="contained"
              size="small"
              startIcon={<DeleteIcon />}
              onClick={onBulkDelete}
              disabled={selectedCount === 0}
              color="error"
              sx={{
                borderRadius: 2,
                textTransform: 'none',
                fontWeight: 600,
                boxShadow: `0 2px 8px ${alpha(theme.palette.error.main, 0.25)}`,
                '&:hover': {
                  boxShadow: `0 4px 12px ${alpha(theme.palette.error.main, 0.35)}`,
                },
              }}
            >
              Delete ({selectedCount})
            </Button>

            <Tooltip title="Clear selection">
              <IconButton
                size="small"
                onClick={onClearSelection}
                sx={{
                  color: 'text.secondary',
                  '&:hover': {
                    bgcolor: alpha(theme.palette.text.secondary, 0.1),
                  },
                }}
              >
                <CloseIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          </Box>
        </Box>
      </Box>
    </Collapse>
  );
};

export default BulkSelectionToolbar;
