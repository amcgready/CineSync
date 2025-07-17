import { AppBar, Box, Toolbar, Typography, IconButton, Avatar, Tooltip, useMediaQuery, useTheme, Chip, alpha } from '@mui/material';
import Brightness4Icon from '@mui/icons-material/Brightness4';
import Brightness7Icon from '@mui/icons-material/Brightness7';
import MenuIcon from '@mui/icons-material/Menu';
import LogoutIcon from '@mui/icons-material/Logout';
import { useAuth } from '../../contexts/AuthContext';
import { useConfig } from '../../contexts/ConfigContext';
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
  const { config } = useConfig();
  const navigate = useNavigate();
  const [bannerImage, setBannerImage] = useState<string>('');

  // Future enhancement ideas:
  // 1. Use Canvas API to crop images client-side to perfect banner dimensions
  // 2. Implement face/object detection to center important visual elements
  // 3. Add user preference for banner style (action-focused vs landscape-focused)
  // 4. Cache processed images locally to reduce API calls

  // Fetch random banner from multiple sources
  useEffect(() => {
    const fetchRandomBanner = async () => {
      try {
        const TMDB_API_KEY = config.tmdbApiKey;
        
        if (!TMDB_API_KEY || TMDB_API_KEY === 'your_tmdb_api_key_here') {
          console.warn('TMDB API key not configured. Banner feature disabled.');
          return;
        }

        // First, get popular content from TMDB to get IDs
        const [moviesResponse, tvResponse] = await Promise.all([
          fetch(`https://api.themoviedb.org/3/movie/popular?api_key=${TMDB_API_KEY}&language=en-US&page=1`),
          fetch(`https://api.themoviedb.org/3/tv/popular?api_key=${TMDB_API_KEY}&language=en-US&page=1`)
        ]);
        
        if (!moviesResponse.ok || !tvResponse.ok) {
          throw new Error('Failed to fetch content');
        }

        const [moviesData, tvData] = await Promise.all([
          moviesResponse.json(),
          tvResponse.json()
        ]);
        
        // Combine movies and TV shows
        const allContent = [
          ...moviesData.results.map((item: any) => ({ ...item, type: 'movie' })),
          ...tvData.results.map((item: any) => ({ ...item, type: 'tv' }))
        ];
        
        // Try to get banner from Fanart.tv first (perfect banner aspect ratios)
        let bannerFound = false;
        let attempts = 0;
        const maxAttempts = 5;
        
        while (!bannerFound && attempts < maxAttempts && allContent.length > 0) {
          const randomContent = allContent[Math.floor(Math.random() * allContent.length)];
          
          try {
            // Get banner from Fanart.tv using TMDB ID
            const FANART_API_KEY = import.meta.env.VITE_FANART_API_KEY || 'your_fanart_api_key_here';
            
            if (FANART_API_KEY === 'your_fanart_api_key_here') {
              console.warn('Fanart.tv API key not configured, skipping to TMDB fallback');
              break;
            }
            
            const fanartUrl = randomContent.type === 'movie' 
              ? `https://webservice.fanart.tv/v3/movies/${randomContent.id}?api_key=${FANART_API_KEY}`
              : `https://webservice.fanart.tv/v3/tv/${randomContent.id}?api_key=${FANART_API_KEY}`;
            
            const fanartResponse = await fetch(fanartUrl);
            
            if (fanartResponse.ok) {
              const fanartData = await fanartResponse.json();
              
              // Try different banner types in order of preference
              const bannerTypes = ['hdclearart', 'clearart', 'tvbanner'];
              let selectedBanner = null;
              
              for (const bannerType of bannerTypes) {
                if (fanartData[bannerType] && fanartData[bannerType].length > 0) {
                  selectedBanner = fanartData[bannerType][0].url;
                  break;
                }
              }
              
              if (selectedBanner) {
                console.log('Fanart.tv Banner:', {
                  title: randomContent.title || randomContent.name,
                  type: randomContent.type,
                  url: selectedBanner,
                  source: 'Fanart.tv (perfect banner aspect ratio)'
                });
                setBannerImage(selectedBanner);
                bannerFound = true;
              }
            }
          } catch (fanartError) {
            console.log('Fanart.tv failed for', randomContent.title || randomContent.name);
          }
          
          attempts++;
        }
        
        // Fallback to TMDB backdrops if no Fanart.tv banner found
        if (!bannerFound && allContent.length > 0) {
          const contentWithBackdrops = allContent.filter((item: any) => item.backdrop_path);
          if (contentWithBackdrops.length > 0) {
            const randomContent = contentWithBackdrops[Math.floor(Math.random() * contentWithBackdrops.length)];
            const backdropUrl = `https://image.tmdb.org/t/p/w1920${randomContent.backdrop_path}`;
            console.log('TMDB Fallback Banner:', { 
              title: randomContent.title || randomContent.name, 
              type: randomContent.type,
              url: backdropUrl,
              source: 'TMDB backdrop (cropped)'
            });
            setBannerImage(backdropUrl);
          }
        }
        
      } catch (error) {
        console.error('Error fetching banner:', error);
      }
    };

    fetchRandomBanner();
    
    // Refresh banner every 5 minutes
    const interval = setInterval(fetchRandomBanner, 5 * 60 * 1000);
    return () => clearInterval(interval);
  }, [config.tmdbApiKey]);

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
        // Ensure fixed positioning works on mobile
        position: 'fixed !important',
        transform: 'none !important',
        WebkitTransform: 'none !important',
        willChange: 'auto',
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
                  width: '100%',
                  height: '100%',
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