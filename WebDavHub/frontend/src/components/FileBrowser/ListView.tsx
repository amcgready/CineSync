import {
  Box,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
  useTheme,
  useMediaQuery,
  Checkbox,
} from '@mui/material';
import { FileItem } from './types';
import { getFileIcon } from './fileUtils';
import FileActionMenu from './FileActionMenu';
import MobileListItem from './MobileListItem';

interface ListViewProps {
  files: FileItem[];
  currentPath: string;
  formatDate: (date?: string) => string;
  onItemClick: (file: FileItem) => void;
  onViewDetails: (file: FileItem, details: any) => void;
  onRename: () => void;
  onDeleted: () => void;
  onError: (error: string) => void;
  onNavigateBack?: () => void;
  selectedFiles?: Set<string>;
  onFileSelect?: (fileName: string, checked: boolean) => void;
  selectionMode?: boolean;
}

export default function ListView({
  files,
  currentPath,
  formatDate,
  onItemClick,
  onViewDetails,
  onRename,
  onDeleted,
  onError,
  onNavigateBack,
  selectedFiles = new Set(),
  onFileSelect,
  selectionMode = false,
}: ListViewProps) {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));

  if (files.length === 0) {
    return (
      <Box sx={{ textAlign: 'center', py: 4 }}>
        <Typography color="text.secondary">
          This folder is empty.
        </Typography>
      </Box>
    );
  }

  if (isMobile) {
    return (
      <Paper
        elevation={3}
        sx={{
          borderRadius: 3,
          overflow: 'hidden',
        }}
      >
        {files.map((file) => (
          <MobileListItem
            key={file.name}
            file={file}
            onItemClick={() => onItemClick(file)}
            formatDate={formatDate}
            isSelected={selectedFiles.has(file.name)}
            onSelect={(checked) => {
              if (onFileSelect) {
                onFileSelect(file.name, checked);
              }
            }}
            showSelection={selectionMode || selectedFiles.size > 0}
            menu={
              <FileActionMenu
                file={file}
                currentPath={currentPath}
                onViewDetails={onViewDetails}
                onRename={onRename}
                onDeleted={onDeleted}
                onError={onError}
                onNavigateBack={onNavigateBack}
              />
            }
          />
        ))}
      </Paper>
    );
  }

  return (
    <TableContainer component={Paper} sx={{
      width: '100%',
      maxWidth: '100vw',
      overflowX: 'auto',
      boxShadow: 3,
      borderRadius: 3,
    }}>
      <Table sx={{
        tableLayout: 'fixed',
        '& td, & th': {
          px: 2,
          py: 1.5,
          '&:first-of-type': { width: (selectionMode || selectedFiles.size > 0) ? '50px' : '50%' },
          '&:nth-of-type(2)': { width: (selectionMode || selectedFiles.size > 0) ? '45%' : '15%' },
          '&:nth-of-type(3)': { width: '15%' },
          '&:nth-of-type(4)': { width: '25%' },
          '&:last-child': { width: '10%' },
        },
      }}>
        <TableHead>
          <TableRow sx={{ bgcolor: 'action.hover' }}>
            {(selectionMode || selectedFiles.size > 0) && (
              <TableCell sx={{ fontWeight: 600, width: '50px' }}>
                <Checkbox
                  checked={selectedFiles.size === files.length && files.length > 0}
                  indeterminate={selectedFiles.size > 0 && selectedFiles.size < files.length}
                  onChange={(e) => {
                    if (onFileSelect) {
                      files.forEach(file => onFileSelect(file.name, e.target.checked));
                    }
                  }}
                  size="small"
                />
              </TableCell>
            )}
            <TableCell sx={{ fontWeight: 600 }}>Name</TableCell>
            <TableCell sx={{ fontWeight: 600 }}>Size</TableCell>
            <TableCell sx={{ fontWeight: 600 }}>Modified</TableCell>
            <TableCell align="right"></TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {files.map((file) => (
            <TableRow
              key={file.name}
              data-file-name={file.name}
              hover
              onClick={() => onItemClick(file)}
              sx={{
                cursor: file.type === 'directory' ? 'pointer' : 'default',
                transition: 'background-color 0.2s',
                '&:hover': { bgcolor: 'action.hover' },
                '&.alphabet-highlight': {
                  backgroundColor: theme.palette.primary.main + '20',
                  animation: 'pulse 2s ease-in-out',
                },
                '@keyframes pulse': {
                  '0%': { backgroundColor: theme.palette.primary.main + '40' },
                  '50%': { backgroundColor: theme.palette.primary.main + '20' },
                  '100%': { backgroundColor: 'transparent' },
                }
              }}
            >
              {(selectionMode || selectedFiles.size > 0) && (
                <TableCell onClick={(e) => e.stopPropagation()}>
                  <Checkbox
                    checked={selectedFiles.has(file.name)}
                    onChange={(e) => {
                      if (onFileSelect) {
                        onFileSelect(file.name, e.target.checked);
                      }
                    }}
                    size="small"
                  />
                </TableCell>
              )}
              <TableCell>
                <Box sx={{ display: 'flex', alignItems: 'center', minWidth: 0 }}>
                  <Box sx={{ mr: 2, display: 'flex' }}>
                    {getFileIcon(file.name, file.type)}
                  </Box>
                  <Typography
                    sx={{
                      fontWeight: 500,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {file.name}
                  </Typography>
                </Box>
              </TableCell>
              <TableCell>{file.type === 'directory' ? '--' : file.size}</TableCell>
              <TableCell>{formatDate(file.modified)}</TableCell>
              <TableCell align="right" onClick={e => e.stopPropagation()}>
                <FileActionMenu
                  file={file}
                  currentPath={currentPath}
                  onViewDetails={onViewDetails}
                  onRename={onRename}
                  onDeleted={onDeleted}
                  onError={onError}
                  onNavigateBack={onNavigateBack}
                />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
} 