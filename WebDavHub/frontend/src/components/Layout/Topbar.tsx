import { AppBar, Box, Toolbar, Typography, IconButton, Avatar, Tooltip, useMediaQuery, useTheme, Chip, alpha } from '@mui/material';
import Brightness4Icon from '@mui/icons-material/Brightness4';
import Brightness7Icon from '@mui/icons-material/Brightness7';
import MenuIcon from '@mui/icons-material/Menu';
import LogoutIcon from '@mui/icons-material/Logout';
import { useAuth } from '../../contexts/AuthContext';
import { useNavigate } from 'react-router-dom';
import { useState, useEffect } from 'react';
import logoImage from '../../assets/logo.png';
import './topbar-fixes.css';

interface TopbarProps {
  toggleTheme: () => void;
  mode: 'light' | 'dark';
  onMenuClick?: () => void;
}

export default function Topbar({ toggleTheme, mode, onMenuClick }: TopbarProps) {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));
  const { logout, user } = useAuth();
  const navigate = useNavigate();
  const [bannerImage, setBannerImage] = useState<string>('');

  // Load random banner from local assets
  useEffect(() => {
    const loadLocalBanner = async () => {
      try {
        // Define available banner images in the topbar folder
        const availableBanners: string[] = [
          // Add banner filenames here when they are added to /src/assets/topbar/
          // 'banner1.jpg',
          // 'banner2.jpg',
          // 'banner3.png',
        ];

        if (availableBanners.length > 0) {
          // Select a random banner from available images
          const randomBanner = availableBanners[Math.floor(Math.random() * availableBanners.length)];
          const bannerUrl = `/src/assets/topbar/${randomBanner}`;
          setBannerImage(bannerUrl);
          console.log('Local banner loaded:', bannerUrl);
        } else {
          // No banners available, use default styling
          setBannerImage('');
          console.log('No local banners available, using default styling');
        }
      } catch (error) {
        console.error('Error loading local banner:', error);
        setBannerImage('');
      }
    };

    loadLocalBanner();
    
    // Refresh banner every 5 minutes (if multiple banners are available)
    const interval = setInterval(loadLocalBanner, 5 * 60 * 1000);
    return () => clearInterval(interval);
  }, []);

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const handleLogoClick = () => {
    navigate('/dashboard');
  };

  return (
    <AppBar
      position="sticky"
      elevation={0}
      sx={{
        backgroundImage: bannerImage 
          ? `linear-gradient(
              ${mode === 'dark' 
                ? `${alpha(theme.palette.background.paper, 0.45)}, ${alpha(theme.palette.background.paper, 0.435)}`
                : `${alpha('#ffffff', 0.45)}, ${alpha('#ffffff', 0.435)}`
              }
            ), url(${bannerImage})`
          : 'none',
        backgroundSize: 'cover', // Cover the entire banner area
        backgroundPosition: 'center 20%', // Smart crop: avoid sky areas, show action/characters
        backgroundRepeat: 'no-repeat',
        backgroundAttachment: 'local', // Ensure background stays within container bounds
        bgcolor: !bannerImage 
          ? (mode === 'dark'
              ? alpha(theme.palette.background.paper, 0.8)
              : alpha('#ffffff', 0.9))
          : 'transparent',
        backdropFilter: 'blur(24px)',
        borderBottom: '1px solid',
        borderColor: alpha(theme.palette.divider, 0.08),
        zIndex: theme.zIndex.drawer + 1,
        top: 0,
        left: 0,
        right: 0,
        overflow: 'hidden', // Clip any overflow from the background image
        // Smooth transition when banner changes
        transition: 'background-image 0.5s ease-in-out',
      }}
    >
      <Toolbar sx={{
        minHeight: 188, // Fixed height to accommodate 180px logo
        px: { xs: 2, sm: 3 },
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center'
      }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: { xs: 1, sm: 2 } }}>
          {onMenuClick && (
            <IconButton
              color="inherit"
              aria-label="open drawer"
              edge="start"
              onClick={onMenuClick}
              size="small"
              sx={{
                mr: 1,
                display: { md: 'none' },
                color: 'text.secondary',
                '&:hover': {
                  bgcolor: alpha(theme.palette.primary.main, 0.08),
                  color: theme.palette.primary.main,
                },
                transition: 'all 0.2s ease-in-out'
              }}
            >
              <MenuIcon />
            </IconButton>
          )}

          <Box
            onClick={handleLogoClick}
            data-topbar-logo
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: { xs: 1, sm: 1.5 },
              cursor: 'pointer',
              borderRadius: 2,
              p: 0.5,
              transition: 'all 0.2s ease-in-out',
              '&:hover': {
                bgcolor: alpha(theme.palette.primary.main, 0.04),
              }
            }}
          >
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                width: 180, // Fixed width to match sidebar (180px on all screen sizes)
                height: 180, // Fixed height to maintain square aspect ratio
                borderRadius: 0, // Remove border radius
                overflow: 'hidden',
                // Removed box shadow and border styling
                transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
                '&:hover': {
                  transform: 'scale(1.02)', // Reduced hover scale
                }
              }}
            >
              <img
                src={logoImage}
                alt="CineSync Logo"
                style={{
                  width: '75%',
                  height: '75%',
                  objectFit: 'contain',
                  display: 'block'
                }}
                onError={(e) => {
                  // Hide the logo and show fallback text if logo fails to load
                  e.currentTarget.style.display = 'none';
                  const textElement = e.currentTarget.closest('[data-topbar-logo]')?.querySelector('[data-fallback-text]') as HTMLElement;
                  if (textElement) {
                    textElement.style.display = 'block';
                  }
                }}
              />
            </Box>

            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Typography
                variant="h6"
                data-fallback-text
                sx={{
                  fontWeight: 600,
                  color: 'text.primary',
                  fontSize: { xs: '1.25rem', sm: '1.35rem' },
                  letterSpacing: '-0.01em',
                  fontFamily: '"Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
                  display: 'none' // Hidden by default, only shown if logo fails to load
                }}
              >
                CineSync
              </Typography>
            </Box>
          </Box>
        </Box>

        <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: { xs: 1, sm: 1.5 }, alignSelf: 'flex-start', mt: 0.5 }}>
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 0.5 }}>
            <Box sx={{
            display: 'flex',
            alignItems: 'center',
            gap: { xs: 0.75, sm: 1 },
            bgcolor: alpha(theme.palette.background.default, 0.4),
            border: `1px solid ${alpha(theme.palette.divider, 0.1)}`,
            borderRadius: 6,
            px: { xs: 1, sm: 1.25 },
            py: 0.5,
            backdropFilter: 'blur(12px)',
            transition: 'all 0.2s ease-in-out',
            '&:hover': {
              bgcolor: alpha(theme.palette.background.default, 0.6),
              borderColor: alpha(theme.palette.divider, 0.2),
            }
          }}>
            <Avatar
              sx={{
                bgcolor: theme.palette.primary.main,
                width: { xs: 28, sm: 30 },
                height: { xs: 28, sm: 30 },
                fontSize: { xs: '0.75rem', sm: '0.8rem' },
                fontWeight: 600,
                transition: 'all 0.2s ease-in-out',
                '&:hover': {
                  transform: 'scale(1.05)',
                }
              }}
            >
              {(user?.username || 'A').charAt(0).toUpperCase()}
            </Avatar>

            {!isMobile && (
              <Typography
                variant="body2"
                sx={{
                  fontWeight: 500,
                  color: 'text.primary',
                  fontSize: '0.875rem',
                  letterSpacing: '0.005em'
                }}
              >
                {user?.username || 'Admin'}
              </Typography>
            )}

            <Tooltip title="Logout" arrow>
              <IconButton
                size="small"
                onClick={handleLogout}
                sx={{
                  color: 'text.secondary',
                  width: 28,
                  height: 28,
                  '&:hover': {
                    bgcolor: alpha(theme.palette.error.main, 0.1),
                    color: 'error.main',
                  },
                  transition: 'all 0.2s ease-in-out'
                }}
              >
                <LogoutIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          </Box>

            <Box sx={{ 
              display: 'flex', 
              alignItems: 'center', 
              gap: 1, 
              mt: 0.25, // Further reduced padding above the chips (was 1.0)
              justifyContent: 'center' // Center the chips
            }}>
              <Tooltip title={mode === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'} arrow>
                <IconButton
                  size="small"
                  onClick={toggleTheme}
                  sx={{
                    color: 'text.secondary',
                    width: 32,
                    height: 32,
                    borderRadius: '50%',
                    '&:hover': {
                      bgcolor: alpha(theme.palette.primary.main, 0.08),
                      color: theme.palette.primary.main,
                    },
                    transition: 'all 0.2s ease-in-out'
                  }}
                >
                  {mode === 'dark' ? <Brightness7Icon fontSize="small" /> : <Brightness4Icon fontSize="small" />}
                </IconButton>
              </Tooltip>

              <Chip
                label="v3.0"
                size="small"
                variant="outlined"
                sx={{
                  height: 20,
                  fontSize: '0.65rem',
                  fontWeight: 600,
                  color: theme.palette.primary.main,
                  borderColor: theme.palette.primary.main,
                  bgcolor: alpha(theme.palette.primary.main, 0.08),
                  '& .MuiChip-label': {
                    px: 0.75
                  },
                  '&:hover': {
                    bgcolor: alpha(theme.palette.primary.main, 0.12),
                  },
                  transition: 'all 0.2s ease-in-out'
                }}
              />
            </Box>
          </Box>
        </Box>
      </Toolbar>
    </AppBar>
  );
}