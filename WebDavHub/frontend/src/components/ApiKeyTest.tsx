import { useEffect, useState } from 'react';
import { useConfig } from '../contexts/ConfigContext';

export default function ApiKeyTest() {
  const { config } = useConfig();
  const [testResult, setTestResult] = useState<any>(null);

  useEffect(() => {
    const runTest = async () => {
      const result = {
        timestamp: new Date().toISOString(),
        configFanartKey: config.fanartApiKey || 'MISSING',
        viteEnvFanartKey: import.meta.env.VITE_FANART_API_KEY || 'MISSING',
        allViteEnvKeys: Object.keys(import.meta.env).filter(k => k.startsWith('VITE_')),
        finalKey: config.fanartApiKey || import.meta.env.VITE_FANART_API_KEY || 'MISSING',
        isValidKey: false,
        fanartTestResult: 'Not tested'
      };

      // Test if we have a valid Fanart.tv API key
      const testKey = result.finalKey;
      if (testKey && testKey !== 'MISSING' && testKey !== 'your_fanart_api_key_here') {
        try {
          const response = await fetch(`https://webservice.fanart.tv/v3/movies/550?api_key=${testKey}`);
          if (response.ok) {
            const data = await response.json();
            result.isValidKey = true;
            result.fanartTestResult = `SUCCESS - ${data.name} found with ${Object.keys(data).length} image types`;
          } else {
            result.fanartTestResult = `FAILED - HTTP ${response.status}`;
          }
        } catch (error) {
          result.fanartTestResult = `ERROR - ${error}`;
        }
      } else {
        result.fanartTestResult = 'No valid API key to test';
      }

      setTestResult(result);
      console.log('🧪 API Key Test Results:', result);
    };

    runTest();
  }, [config]);

  if (!testResult) return <div>Running API key test...</div>;

  return (
    <div style={{
      position: 'fixed',
      top: '10px',
      right: '10px',
      background: 'rgba(0,0,0,0.8)',
      color: 'white',
      padding: '10px',
      borderRadius: '5px',
      fontSize: '12px',
      maxWidth: '400px',
      zIndex: 9999
    }}>
      <h4>🧪 API Key Test</h4>
      <div><strong>Config Key:</strong> {testResult.configFanartKey === 'MISSING' ? '❌ Missing' : '✅ Present'}</div>
      <div><strong>Vite Env Key:</strong> {testResult.viteEnvFanartKey === 'MISSING' ? '❌ Missing' : '✅ Present'}</div>
      <div><strong>Final Key:</strong> {testResult.finalKey === 'MISSING' ? '❌ Missing' : '✅ Present'}</div>
      <div><strong>Vite Env Vars:</strong> {testResult.allViteEnvKeys.join(', ') || 'None'}</div>
      <div><strong>Fanart.tv Test:</strong> {testResult.fanartTestResult}</div>
      <div><strong>Valid Key:</strong> {testResult.isValidKey ? '✅ Yes' : '❌ No'}</div>
    </div>
  );
}
