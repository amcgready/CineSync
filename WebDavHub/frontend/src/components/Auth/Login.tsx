import { useState } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import {
  Box,
  Button,
  Container,
  TextField,
  Typography,
  Paper,
  InputAdornment,
  IconButton,
  Alert,
  Tooltip,
} from '@mui/material';
import {
  Visibility,
  VisibilityOff,
  Login as LoginIcon,
} from '@mui/icons-material';
import Brightness4Icon from '@mui/icons-material/Brightness4';
import Brightness7Icon from '@mui/icons-material/Brightness7';
import { useAuth } from '../../contexts/AuthContext';
import { motion, AnimatePresence } from 'framer-motion';
import axios from 'axios';
import logo from '../../assets/logo.png';

const MotionPaper = motion(Paper);

interface LocationState {
  from?: {
    pathname: string;
  };
}

export default function Login({ toggleTheme, mode }: { toggleTheme: () => void; mode: 'light' | 'dark' }) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();
  const { login } = useAuth();

  // Get the return URL from location state or default to dashboard
  const from = (location.state as LocationState)?.from?.pathname || '/dashboard';

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      await login(username, password);
      // Navigate to the return URL or dashboard
      navigate(from, { replace: true });
    } catch (err) {
      if (axios.isAxiosError(err)) {
        if (err.response?.status === 401) {
          setError('Invalid username or password');
        } else if (err.response?.status === 403) {
          setError('Access denied. Please check your credentials.');
        } else {
          setError('An error occurred. Please try again.');
        }
      } else {
        setError('An unexpected error occurred. Please try again.');
      }
      setPassword('');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Container component="main" maxWidth="xs">
      <Box
        sx={{
          minHeight: '100vh',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <motion.div
          initial={{ opacity: 0, y: 40 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.7, ease: 'easeOut' }}
          style={{ width: '100%' }}
        >
          <MotionPaper
            elevation={mode === 'dark' ? 24 : 0}
            sx={{
              padding: 4,
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              width: '100%',
              background: mode === 'dark' 
                ? 'rgba(255, 255, 255, 0.05)'
                : 'rgba(255, 255, 255, 0.85)',
              backdropFilter: 'blur(10px)',
              borderRadius: '16px',
              border: mode === 'dark' 
                ? '1px solid rgba(255, 255, 255, 0.1)'
                : '1px solid rgba(0,0,0,0.07)',
              position: 'relative',
              boxShadow: mode === 'dark'
                ? undefined
                : '0 4px 24px 0 rgba(0,0,0,0.10), 0 1.5px 4px 0 rgba(0,0,0,0.08)',
            }}
            initial={false}
            animate={true}
          >
            {/* Dark/Light mode switch */}
            <Box sx={{ alignSelf: 'flex-end', mb: 1, position: 'absolute', top: 12, right: 12 }}>
              <Tooltip title={mode === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}>
                <IconButton onClick={toggleTheme} color="inherit">
                  {mode === 'dark' ? <Brightness7Icon /> : <Brightness4Icon />}
                </IconButton>
              </Tooltip>
            </Box>
            <motion.div
              initial={{ scale: 0 }}
              animate={{ scale: 1 }}
              transition={{ delay: 0.2, type: "spring", stiffness: 200 }}
            >
              <img src={logo} alt="CineSync Logo" style={{ display: 'block', margin: '0 auto 20px auto', maxWidth: '160px', width: '100%', height: 'auto' }} />
              <Typography
                component="h1"
                variant="h4"
                sx={{
                  mb: 3,
                  fontWeight: 700,
                  background: 'linear-gradient(45deg, #2196F3 30%, #21CBF3 90%)',
                  backgroundClip: 'text',
                  textFillColor: 'transparent',
                  WebkitBackgroundClip: 'text',
                  WebkitTextFillColor: 'transparent',
                }}
              >
                CineSync Login
              </Typography>
            </motion.div>

            <Box component="form" onSubmit={handleSubmit} sx={{ width: '100%' }}>
              <motion.div
                initial={{ x: -20, opacity: 0 }}
                animate={{ x: 0, opacity: 1 }}
                transition={{ delay: 0.3 }}
              >
                <TextField
                  margin="normal"
                  required
                  fullWidth
                  id="username"
                  label="Username"
                  name="username"
                  autoComplete="username"
                  autoFocus
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  disabled={loading}
                  error={!!error}
                  sx={{
                    '& .MuiOutlinedInput-root': {
                      '& fieldset': {
                        borderColor: 'rgba(255, 255, 255, 0.2)',
                      },
                      '&:hover fieldset': {
                        borderColor: 'rgba(255, 255, 255, 0.3)',
                      },
                    },
                  }}
                />
              </motion.div>

              <motion.div
                initial={{ x: -20, opacity: 0 }}
                animate={{ x: 0, opacity: 1 }}
                transition={{ delay: 0.4 }}
              >
                <TextField
                  margin="normal"
                  required
                  fullWidth
                  name="password"
                  label="Password"
                  type={showPassword ? 'text' : 'password'}
                  id="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  disabled={loading}
                  error={!!error}
                  InputProps={{
                    endAdornment: (
                      <InputAdornment position="end">
                        <IconButton
                          aria-label="toggle password visibility"
                          onClick={() => setShowPassword(!showPassword)}
                          edge="end"
                          disabled={loading}
                        >
                          {showPassword ? <VisibilityOff /> : <Visibility />}
                        </IconButton>
                      </InputAdornment>
                    ),
                  }}
                  sx={{
                    '& .MuiOutlinedInput-root': {
                      '& fieldset': {
                        borderColor: 'rgba(255, 255, 255, 0.2)',
                      },
                      '&:hover fieldset': {
                        borderColor: 'rgba(255, 255, 255, 0.3)',
                      },
                    },
                  }}
                />
              </motion.div>

              <AnimatePresence>
                {error && (
                  <motion.div
                    key="error-alert"
                    initial={{ opacity: 0, y: -10 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: -10 }}
                    transition={{ duration: 0.3 }}
                  >
                    <Alert 
                      severity="error" 
                      sx={{ 
                        mt: 2, 
                        width: '100%',
                        animation: 'shake 0.5s cubic-bezier(.36,.07,.19,.97) both',
                        '@keyframes shake': {
                          '10%, 90%': { transform: 'translate3d(-1px, 0, 0)' },
                          '20%, 80%': { transform: 'translate3d(2px, 0, 0)' },
                          '30%, 50%, 70%': { transform: 'translate3d(-4px, 0, 0)' },
                          '40%, 60%': { transform: 'translate3d(4px, 0, 0)' },
                        },
                      }}
                    >
                      {error}
                    </Alert>
                  </motion.div>
                )}
              </AnimatePresence>

              <motion.div
                initial={{ y: 20, opacity: 0 }}
                animate={{ y: 0, opacity: 1 }}
                transition={{ delay: 0.5 }}
              >
                <Button
                  type="submit"
                  fullWidth
                  variant="contained"
                  disabled={loading}
                  sx={{
                    mt: 3,
                    mb: 2,
                    py: 1.5,
                    background: 'linear-gradient(45deg, #2196F3 30%, #21CBF3 90%)',
                    '&:hover': {
                      background: 'linear-gradient(45deg, #1976D2 30%, #1E88E5 90%)',
                    },
                  }}
                  startIcon={<LoginIcon />}
                >
                  {loading ? 'Signing in...' : 'Sign In'}
                </Button>
              </motion.div>
            </Box>
          </MotionPaper>
        </motion.div>
      </Box>
    </Container>
  );
} 