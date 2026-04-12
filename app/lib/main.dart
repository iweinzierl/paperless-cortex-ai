import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import 'package:app/providers/auth_provider.dart';
import 'package:app/screens/app_shell.dart';
import 'package:app/screens/login_screen.dart';
import 'package:app/services/api_service.dart';
import 'package:app/theme.dart';

void main() {
  final apiService = ApiService();
  runApp(MainApp(apiService: apiService));
}

class MainApp extends StatelessWidget {
  const MainApp({super.key, required this.apiService});

  final ApiService apiService;

  @override
  Widget build(BuildContext context) {
    return MultiProvider(
      providers: [
        Provider<ApiService>.value(value: apiService),
        ChangeNotifierProvider<AuthProvider>(
          create: (_) => AuthProvider(apiService)..init(),
        ),
      ],
      child: MaterialApp(
        title: 'Cortex Graphite Mobile',
        debugShowCheckedModeBanner: false,
        theme: buildAppTheme(),
        home: const _AppEntry(),
      ),
    );
  }
}

class _AppEntry extends StatelessWidget {
  const _AppEntry();

  @override
  Widget build(BuildContext context) {
    final auth = context.watch<AuthProvider>();

    if (!auth.initialized) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }

    if (auth.isAuthenticated) {
      return const AppShell();
    }

    return const LoginScreen();
  }
}
