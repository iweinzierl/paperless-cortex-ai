import 'package:flutter/foundation.dart';

import 'package:app/services/api_service.dart';

class AuthProvider extends ChangeNotifier {
  AuthProvider(this.apiService);

  final ApiService apiService;

  bool _initialized = false;
  bool _isAuthenticated = false;
  String? _username;

  bool get initialized => _initialized;
  bool get isAuthenticated => _isAuthenticated;
  String? get username => _username;

  Future<void> init() async {
    await apiService.init();
    final currentToken = apiService.token;

    if (currentToken != null && currentToken.isNotEmpty) {
      try {
        final session = await apiService.getMe();
        _isAuthenticated = true;
        _username = session.username.trim().isEmpty ? null : session.username;
      } catch (_) {
        await apiService.logout();
        _isAuthenticated = false;
        _username = null;
      }
    }

    _initialized = true;
    notifyListeners();
  }

  Future<void> login(String username, String password) async {
    final session = await apiService.login(username, password);
    _isAuthenticated = true;
    _username = session.username.trim().isEmpty
        ? username.trim()
        : session.username;
    notifyListeners();
  }

  Future<void> logout() async {
    await apiService.logout();
    _isAuthenticated = false;
    _username = null;
    notifyListeners();
  }
}
