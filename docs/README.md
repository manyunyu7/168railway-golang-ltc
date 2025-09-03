# 168Railway Go Live Tracking API - Documentation Index

## 📚 Complete Documentation Guide

Welcome to the comprehensive documentation for the 168Railway Go Live Tracking API - a high-performance Golang replacement for Laravel live tracking endpoints.

**Production URL**: https://go-ltc.trainradar35.com/

---

## 🚀 Quick Start

### New to the API?
1. **[API Overview](../README.md)** - Start here for project overview
2. **[Authentication Guide](guides/AUTHENTICATION_GUIDE.md)** - Get your tokens working
3. **[Mobile Developer Guide](guides/MOBILE_DEVELOPER_GUIDE.md)** - Mobile app integration

### Want to add features?
1. **[Development Setup](../CLAUDE.md)** - Development environment and commands
2. **[Testing Guide](development/TESTING_GUIDE.md)** - Testing procedures

---

## 📋 API Documentation

### Core APIs
| Documentation | Description | Audience |
|---------------|-------------|----------|
| **[Spotter Location API](api/SPOTTER_API.md)** | Real-time user location tracking for train spotters | Frontend Developers |
| **[Version Management API](api/VERSION_API_GUIDE.md)** | App version checking and update management | Mobile Developers |
| **[API Migration Guide](api/API_MIGRATION_GUIDE.md)** | Migrating from legacy endpoints | All Developers |

### Admin APIs
| Documentation | Description | Audience |
|---------------|-------------|----------|
| **[Admin API Documentation](admin/admin_api_documentation.html)** | Admin interface and session management | Admin Users |

---

## 🛠 Integration Guides

### Frontend Integration
| Guide | Description | Technology |
|-------|-------------|------------|
| **[Spotter Integration Guide](guides/SPOTTER_INTEGRATION_GUIDE.md)** | Complete implementation examples | React, Vue, Laravel, Flutter |
| **[Authentication Guide](guides/AUTHENTICATION_GUIDE.md)** | Token management and security | All Platforms |
| **[Mobile Developer Guide](guides/MOBILE_DEVELOPER_GUIDE.md)** | Mobile-specific integration | Flutter, React Native |

### Specialized Features
| Guide | Description | Use Case |
|-------|-------------|----------|
| **[Tile Proxy Guide](guides/TILE_PROXY_GUIDE.md)** | CartoDB map tiles integration | Map Applications |
| **[Trip Saving Documentation](guides/TRIP_SAVING_DOCUMENTATION.md)** | GPS trip recording system | Tracking Features |

---

## 🔧 Development Documentation

### For Contributors
| Documentation | Description | Audience |
|---------------|-------------|----------|
| **[Development Setup](../CLAUDE.md)** | Environment setup, commands, deployment | Developers |
| **[Testing Guide](development/TESTING_GUIDE.md)** | Testing procedures and best practices | QA, Developers |
| **[Version Management](development/VERSION_MANAGEMENT.md)** | Version control and release management | DevOps |

### Technical Notes
| Documentation | Description | Audience |
|---------------|-------------|----------|
| **[Race Conditions Fixed](development/RACE_CONDITIONS_FIXED.md)** | Technical fixes and improvements | Senior Developers |

---

## 📊 Feature Overview

### ✅ Currently Available Features

#### **Real-time Tracking**
- **Live GPS Tracking**: Real-time location updates with S3 storage
- **Train Data**: Live train positions and passenger information
- **WebSocket Support**: Real-time updates with `wss://go-ltc.trainradar35.com/ws/trains`

#### **Spotter Location System** 🆕
- **User Presence**: Track users actively viewing the map
- **Redis Caching**: 30-second cache updates, 5-minute auto-cleanup
- **Scalable**: Supports 30,000+ concurrent users
- **Endpoints**: `POST /api/spotters/heartbeat`, `GET /api/spotters/active`

#### **Authentication**
- **Laravel Sanctum**: Secure token-based authentication
- **Multi-platform**: Web and mobile app support
- **Token Management**: Creation, rotation, and revocation

#### **Admin Interface**
- **Web Dashboard**: Modern, mobile-responsive admin interface
- **Session Management**: Real-time monitoring and control
- **User Management**: Comprehensive user administration

#### **Version Management**
- **App Version API**: Check for updates and enforce minimum versions
- **Update Notifications**: Automatic update prompts
- **Platform Support**: iOS and Android version control

#### **Infrastructure**
- **High Performance**: Golang-based for optimal speed
- **Redis Integration**: Caching and real-time data
- **S3 Storage**: Reliable data storage with IDCloudHost
- **Health Monitoring**: `/health` endpoint with system status

---

## 🌐 API Endpoints Quick Reference

### Public Endpoints
```
GET  /health                    - System health check
GET  /api/active-train-list     - Active trains (replaces S3 access)
GET  /api/train/{number}        - Individual train data
GET  /api/app-version           - App version information
POST /api/check-version         - Version compatibility check
GET  /api/spotters/active       - Active train spotters
WSS  /ws/trains                 - Real-time train updates
```

### Authenticated Endpoints (Require Sanctum Token)
```
# Live Tracking
GET  /api/mobile/live-tracking/active-session
POST /api/mobile/live-tracking/start
POST /api/mobile/live-tracking/update
POST /api/mobile/live-tracking/heartbeat
POST /api/mobile/live-tracking/recover
POST /api/mobile/live-tracking/stop

# Spotter Location
POST /api/spotters/heartbeat    - Send location while viewing map

# Admin (Admin role required)
GET  /admin/api/sessions        - List tracking sessions
POST /admin/api/sessions/terminate/{id} - Terminate session
```

---

## 🏗 Architecture Overview

```
Frontend Applications (Web/Mobile)
    ↓
Authentication (Laravel Sanctum)
    ↓
Go API Server (Gin Framework)
    ↓
┌─────────────────┬─────────────────┐
│ Redis (Cache)   │ MySQL (Data)    │
│ - Sessions      │ - Users         │
│ - Spotters      │ - Tokens        │ 
│ - Train Data    │ - Trips         │
└─────────────────┴─────────────────┘
    ↓
S3 Storage (IDCloudHost)
- Train position files
- GPS tracking data
```

---

## 📱 Supported Platforms

### Mobile Applications
- **Flutter**: Complete Dart examples with Geolocator
- **React Native**: JavaScript integration examples
- **Native iOS/Android**: RESTful API compatible

### Web Applications  
- **React/Next.js**: Modern React hooks implementation
- **Vue.js**: Composition API examples
- **Laravel Blade**: Server-side integration
- **Vanilla JavaScript**: Framework-agnostic examples

### Backend Integration
- **Laravel PHP**: Native Sanctum integration
- **Node.js**: Express.js API examples
- **Any REST Client**: Standard HTTP API

---

## 🔍 Common Use Cases

### For Train Enthusiasts
1. **Track Live Trains** - Real-time positions and passenger data
2. **Share Location** - Show where you're spotting trains
3. **Community Features** - See other active train spotters
4. **Save Journeys** - Record and save your train trips

### For Developers
1. **Mobile Apps** - Integrate live tracking and spotter features
2. **Web Dashboards** - Display real-time train information  
3. **Admin Tools** - Manage users and tracking sessions
4. **Map Applications** - Show trains and spotter locations

### For Operators
1. **Monitor Usage** - Track active sessions and users
2. **Manage System** - Admin interface for session control
3. **Version Control** - Enforce app updates and compatibility
4. **Performance Monitoring** - Health checks and system status

---

## 🆘 Support & Resources

### Getting Help
- **GitHub Issues**: [Report bugs or request features](https://github.com/manyunyu7/168railway-golang-ltc/issues)
- **API Health**: Monitor at `https://go-ltc.trainradar35.com/health`
- **Documentation**: Always up-to-date in this repository

### Development Resources
- **Environment Setup**: See `CLAUDE.md` for complete setup instructions
- **Deployment Guide**: Production deployment procedures included
- **Testing Framework**: Comprehensive testing documentation available

### Community
- **Active Development**: Regular updates and feature additions
- **Open Source**: Public repository for transparency
- **Production Ready**: Currently serving live traffic

---

## 📈 Changelog & Updates

### Latest Updates (September 2025)
- ✅ **Spotter Location System**: Real-time user presence tracking
- ✅ **Enhanced Authentication**: Comprehensive Sanctum integration
- ✅ **Improved Documentation**: Complete reorganization and examples
- ✅ **Performance Optimizations**: Redis caching and scaling improvements

### Recent Features
- ✅ **Admin Interface**: Web-based administration dashboard
- ✅ **WebSocket Support**: Real-time train data streaming
- ✅ **Version Management**: App update system
- ✅ **Trip Saving**: Enhanced GPS journey recording

---

*This documentation is maintained alongside the codebase and is always up-to-date with the latest API changes.*

**Last Updated**: September 3, 2025  
**API Version**: 1.0.0  
**Production URL**: https://go-ltc.trainradar35.com/